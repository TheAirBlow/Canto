// Package importer runs bulk listen-history imports through the same ingest.Service live ingest uses.
package importer

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"Canto/internal/config"
	"Canto/internal/db"
	"Canto/internal/ingest"
)

// Manager owns the shared import worker pool and per-format parsers.
type Manager struct {
	dbPool   *pgxpool.Pool
	queries  *db.Queries
	ingest   *ingest.Service
	defaults config.ProcessorsConfig
	sem      chan struct{}
	dataDir  string
	formats  map[db.ImportService]Format
}

// NewManager builds a Manager backed by queries/ingestService, with a shared pool sized workers.
func NewManager(dbPool *pgxpool.Pool, queries *db.Queries, ingestService *ingest.Service, defaults config.ProcessorsConfig, workers int, dataDir string) *Manager {
	return &Manager{
		dbPool: dbPool, queries: queries, ingest: ingestService, defaults: defaults,
		sem: make(chan struct{}, workers), dataDir: dataDir, formats: defaultFormats(),
	}
}

// RequeueStale resets any job left "running" by a previous crash back to "queued", and resumes it.
func (m *Manager) RequeueStale(ctx context.Context) error {
	jobs, err := m.queries.ResetStaleRunningImportJobs(ctx)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		slog.Warn("importer: requeuing stale running job", "id", job.ID)
		m.runAsync(job)
	}
	return nil
}
