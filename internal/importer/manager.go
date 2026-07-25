// Package importer runs bulk listen-history imports through the same ingest.Service live ingest uses.
package importer

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"Canto/internal/config"
	"Canto/internal/db"
	"Canto/internal/ingest"
	"Canto/internal/search"
	"Canto/internal/stats"
)

// Manager owns the shared import worker pool and per-format parsers.
type Manager struct {
	ctx      context.Context // canceled on graceful shutdown; every running job watches it to pause cleanly
	wg       sync.WaitGroup  // tracks in-flight runJob calls, so shutdown can wait for them to pause/finish
	runMu    sync.Mutex      // held for a job's entire run, so only one bulk import job runs at a time
	dbPool   *pgxpool.Pool
	queries  *db.Queries
	ingest   *ingest.Service
	search   *search.Client
	stats    *stats.Engine
	defaults config.ProcessorsConfig
	sem      chan struct{}
	dataDir  string
	formats  map[ImportService]Format
}

// NewManager builds a Manager backed by queries/ingestService, with a shared pool sized workers; canceling ctx signals every running job to pause rather than abort mid-write.
func NewManager(ctx context.Context, dbPool *pgxpool.Pool, queries *db.Queries, ingestService *ingest.Service, searchClient *search.Client, statsEngine *stats.Engine, defaults config.ProcessorsConfig, workers int, dataDir string) *Manager {
	return &Manager{
		ctx: ctx, dbPool: dbPool, queries: queries, ingest: ingestService, search: searchClient, stats: statsEngine, defaults: defaults,
		sem: make(chan struct{}, workers), dataDir: dataDir, formats: defaultFormats(),
	}
}

// Wait blocks until every in-flight import job has paused or finished; call during shutdown so the process doesn't exit mid-pause.
func (m *Manager) Wait() {
	m.wg.Wait()
}

// ResumeInterruptedJobs fails+deletes any job still "running" (an unclean shutdown, so its progress can't be trusted) and resumes every "queued" or "paused" job from its last recorded progress.
func (m *Manager) ResumeInterruptedJobs(ctx context.Context) error {
	interrupted, err := m.queries.FailInterruptedImportJobs(ctx)
	if err != nil {
		return err
	}
	for _, job := range interrupted {
		slog.Warn("importer: job was running during a previous shutdown, marking failed", "id", job.ID)
		if err := os.Remove(m.finalPath(job.ID, job.Filename)); err != nil && !os.IsNotExist(err) {
			slog.Warn("importer: remove file for failed job failed", "id", job.ID, "err", err)
		}
	}

	resumable, err := m.queries.ListResumableImportJobs(ctx)
	if err != nil {
		return err
	}
	for _, job := range resumable {
		slog.Info("importer: resuming job", "id", job.ID, "from", job.ProcessedItems)
		m.runAsync(job, int(job.ProcessedItems))
	}
	return nil
}
