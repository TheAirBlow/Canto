package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"

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
	"Canto/internal/search"
	"Canto/internal/source"
)

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

// run initializes everything serves HTTP until the process exits.
func run() error {
	ctx := context.Background()

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
	service := ingest.NewService(registry, matchers, engine, queries)

	importManager := importer.NewManager(pool, queries, service, cfg.Processors, cfg.Import.Workers, config.DataDir)
	if err := importManager.RequeueStale(ctx); err != nil {
		slog.Warn("importer: requeue stale jobs failed", "err", err)
	}

	exportService := export.NewService(queries)

	apiServer := api.NewServer(api.Deps{
		Queries: queries, Pool: pool, Auth: cfg.Auth, Defaults: cfg.Processors,
		IngestDefaults: cfg.Ingest, Providers: cfg.Providers, Ingest: service,
		Search: searchClient, Importer: importManager, Export: exportService,
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
	slog.Info("canto listening", "addr", addr)
	return http.ListenAndServe(addr, mux)
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
