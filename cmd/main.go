package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"Canto/internal/api"
	"Canto/internal/auth"
	"Canto/internal/config"
	"Canto/internal/correlate"
	"Canto/internal/db"
	"Canto/internal/export"
	"Canto/internal/importer"
	"Canto/internal/ingest"
	"Canto/internal/logging"
	"Canto/internal/refresh"
	"Canto/internal/rollup"
	"Canto/internal/search"
	"Canto/internal/source"
	"Canto/internal/stats"
)

// shutdownTimeout bounds how long graceful shutdown waits for in-flight HTTP requests and import jobs to wind down.
const shutdownTimeout = 30 * time.Second

// bootstrapAdminUsername/Password are the default admin account's credentials.
const (
	bootstrapAdminUsername = "admin"
	bootstrapAdminPassword = "changeme"
)

// main runs the Canto server and logs a fatal error on failure.
func main() {
	if err := run(); err != nil {
		logging.Fatal("canto exited", "err", err)
	}
}

// run initializes everything, serves HTTP until a shutdown signal arrives, then drains cleanly.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(config.DataDir)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	logging.Init(cfg.Server.Debug, cfg.Server.Trace)

	sqlDB, err := sql.Open("pgx", cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("db: open: %w", err)
	}
	defer sqlDB.Close()
	if err := db.Migrate(sqlDB); err != nil {
		return fmt.Errorf("db: migrate: %w", err)
	}

	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("db: pool: %w", err)
	}
	defer pool.Close()
	queries := db.New(pool)

	if err := queries.DeleteAllNowPlaying(ctx); err != nil {
		return fmt.Errorf("db: clear now playing: %w", err)
	}

	if err := bootstrapAdmin(ctx, queries); err != nil {
		return fmt.Errorf("db: bootstrap admin: %w", err)
	}

	registry := source.NewRegistry(
		source.NewYouTubeProcessor(),
		source.NewMusicBrainzProcessor(cfg.Providers.MusicBrainzURL, cfg.Providers.MusicBrainzRateLimit),
		source.NewLastFMProcessor(cfg.Providers.LastFMAPIKey),
		source.NewDeezerProcessor(),
		source.NewSpotifyProcessor(),
	)
	searchClient := search.NewClient(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey, queries)
	if searchClient.Enabled() {
		if err := searchClient.EnsureIndexes(ctx); err != nil {
			slog.Warn("search: ensure indexes failed", "err", err)
		}
	}
	go searchClient.Run(ctx)
	matchers := correlate.NewMatcherRegistry(
		correlate.NewExactMatcher(queries),
		correlate.NewMeilisearchMatcher(searchClient),
	)
	engine := correlate.NewEngine(pool, searchClient)

	// rollupWriter's ctx outlives the shutdown signal until importManager.Wait() returns.
	rollupCtx, stopRollup := context.WithCancel(context.Background())
	defer stopRollup()
	rollupWriter := rollup.NewWriter(queries, cfg.Rollup.FlushInterval)
	rollupDone := make(chan struct{})
	go func() {
		defer close(rollupDone)
		rollupWriter.Run(rollupCtx)
	}()

	service := ingest.NewService(registry, matchers, engine, queries, rollupWriter)

	importManager := importer.NewManager(ctx, pool, queries, service, cfg.Processors, cfg.Import.Workers, config.DataDir)
	if err := importManager.ResumeInterruptedJobs(ctx); err != nil {
		slog.Warn("importer: resume interrupted jobs failed", "err", err)
	}

	exportService := export.NewService(queries)
	statsEngine := stats.NewEngine(queries, cfg.Stats.RegenInterval)
	go statsEngine.Run(ctx)

	apiServer := api.NewServer(api.Deps{
		Queries: queries, Pool: pool, Auth: cfg.Auth, Defaults: cfg.Processors,
		IngestDefaults: cfg.Ingest, Providers: cfg.Providers, Ingest: service,
		Search: searchClient, Importer: importManager, Export: exportService, Stats: statsEngine,
	})

	refreshWorker := refresh.NewWorker(
		refresh.Config{Interval: cfg.Refresh.Interval, Entities: cfg.Refresh.Entities},
		queries, registry, engine, matchers.Ordered(cfg.Processors.MatcherOrder), cfg.Processors.Normalize,
	)
	if refreshWorker.Enabled() {
		go refreshWorker.Run(ctx)
	}

	mux := http.NewServeMux()
	apiServer.Register(mux)

	addr := fmt.Sprintf("%s:%d", cfg.Server.BindAddr, cfg.Server.Port)
	srv := &http.Server{Addr: addr, Handler: mux}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("canto listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		slog.Info("canto shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("http server shutdown failed", "err", err)
	}
	importManager.Wait()

	stopRollup()
	<-rollupDone
	slog.Info("canto stopped")
	return nil
}

// bootstrapAdmin creates the default admin/changeme account if no admin exists yet.
func bootstrapAdmin(ctx context.Context, queries *db.Queries) error {
	count, err := queries.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := auth.HashPassword(bootstrapAdminPassword)
	if err != nil {
		return err
	}
	if _, err := queries.CreateAdminUser(ctx, db.CreateAdminUserParams{
		Username: bootstrapAdminUsername, PasswordHash: hash,
	}); err != nil {
		return err
	}
	slog.Warn("no admin account existed, created default admin -- change its password immediately",
		"username", bootstrapAdminUsername, "password", bootstrapAdminPassword)
	return nil
}
