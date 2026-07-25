package importer

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"

	"Canto/internal/db"
	"Canto/internal/ingest"
	"Canto/internal/rollup"
)

// runTimeout bounds one entire import job, independent of the request that triggered it.
const runTimeout = 6 * time.Hour

// runAsync starts job's processing in the background, resuming from startIdx.
func (m *Manager) runAsync(job db.ImportJob, startIdx int) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runJob(job, startIdx)
	}()
}

// runJob parses job's file from startIdx onward and submits every entry through the shared worker pool.
func (m *Manager) runJob(job db.ImportJob, startIdx int) {
	m.runMu.Lock()
	defer m.runMu.Unlock()

	jobCtx, cancel := context.WithTimeout(m.ctx, runTimeout)
	defer cancel()

	slog.Info("importer: job started", "id", job.ID, "service", job.Service, "filename", job.Filename, "start_idx", startIdx)
	if err := m.queries.StartImportJob(jobCtx, job.ID); err != nil {
		slog.Error("importer: mark job running failed", "id", job.ID, "err", err)
		return
	}

	format, ok := m.formats[ImportService(job.Service)]
	if !ok {
		m.finishJob(context.Background(), job.ID, db.ImportStatusFailed, "unsupported format")
		return
	}
	file, err := os.Open(m.finalPath(job.ID, job.Filename))
	if err != nil {
		m.finishJob(context.Background(), job.ID, db.ImportStatusFailed, err.Error())
		return
	}
	defer file.Close()

	settings, err := m.resolveSettings(jobCtx, job.UserID)
	if err != nil {
		m.finishJob(context.Background(), job.ID, db.ImportStatusFailed, err.Error())
		return
	}

	entries := make(chan ingest.ListenInput)
	total, err := format.Parse(jobCtx, file, entries, startIdx)
	if err != nil {
		m.finishJob(context.Background(), job.ID, db.ImportStatusFailed, err.Error())
		return
	}
	if err := m.queries.SetImportJobTotal(context.Background(), db.SetImportJobTotalParams{ID: job.ID, TotalItems: int32(total)}); err != nil {
		slog.Warn("importer: set total items failed", "id", job.ID, "err", err)
	}

	var wg sync.WaitGroup
	dispatched := startIdx
	paused := false
consume:
	for {
		select {
		case in, ok := <-entries:
			if !ok {
				break consume
			}
			in.UserID = job.UserID
			dispatched++
			wg.Add(1)
			m.sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-m.sem }()
				m.processEntry(context.Background(), job.ID, in, settings)
			}()
		case <-m.ctx.Done():
			paused = true
			break consume
		}
	}
	wg.Wait()

	if paused {
		slog.Info("importer: job paused for shutdown", "id", job.ID, "dispatched", dispatched)
		if err := m.queries.PauseImportJob(context.Background(), job.ID); err != nil {
			slog.Error("importer: pause job failed", "id", job.ID, "err", err)
		}
		return
	}

	if err := rollup.ReconcileUserState(context.Background(), m.queries, job.UserID); err != nil {
		slog.Error("importer: reconcile listen state failed", "id", job.ID, "err", err)
	}
	if err := m.stats.InvalidateUser(context.Background(), job.UserID); err != nil {
		slog.Error("importer: stats cache invalidate failed", "id", job.ID, "err", err)
	}
	if err := m.search.DrainIndexing(context.Background()); err != nil {
		slog.Warn("importer: drain search indexing failed", "id", job.ID, "err", err)
	}
	m.finishJob(context.Background(), job.ID, db.ImportStatusCompleted, "")
}

// processEntry submits one parsed listen and records the outcome in job's progress counters.
func (m *Manager) processEntry(ctx context.Context, jobID int64, in ingest.ListenInput, settings ingest.ProcessorSettings) {
	progress := db.IncrementImportProgressParams{ID: jobID}

	switch {
	case in.SongName == "" && in.OriginURL == "":
		progress.Skipped = 1
	default:
		if _, err := m.ingest.SubmitListen(ctx, in, settings, false, true); err != nil {
			slog.Warn("importer: submit listen failed", "job", jobID, "err", err)
			progress.Failed = 1
		} else {
			progress.Imported = 1
		}
	}

	if err := m.queries.IncrementImportProgress(ctx, progress); err != nil {
		slog.Error("importer: update progress failed", "job", jobID, "err", err)
	}
}

// finishJob marks jobID with its terminal status, unless it was already cancelled while running.
func (m *Manager) finishJob(ctx context.Context, jobID int64, status db.ImportStatus, errMsg string) {
	current, err := m.queries.GetImportJobByID(ctx, jobID)
	if err == nil && current.Status == db.ImportStatusCancelled {
		slog.Info("importer: job was cancelled, not overwriting", "id", jobID)
		return
	}

	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	if err := m.queries.FinishImportJob(ctx, db.FinishImportJobParams{ID: jobID, Status: status, ErrorMessage: errPtr}); err != nil {
		slog.Error("importer: finish job failed", "id", jobID, "err", err)
		return
	}
	slog.Info("importer: job finished", "id", jobID, "status", status)
}

// settingsDoc is the subset of a user's stored settings importer needs to resolve correlation behavior.
type settingsDoc struct {
	LinkProcessors     []string `json:"link_processors"`
	FallbackProcessors []string `json:"fallback_processors"`
	FuzzyMatchers      []string `json:"fuzzy_matchers"`
}

// resolveSettings loads userID's stored processor settings, falling back to Canto's configured defaults.
func (m *Manager) resolveSettings(ctx context.Context, userID int64) (ingest.ProcessorSettings, error) {
	row, err := m.queries.GetUserSettings(ctx, userID)
	if err != nil {
		return ingest.ProcessorSettings{
			LinkOrder: m.defaults.LinkOrder, FallbackOrder: m.defaults.FallbackOrder,
			MatcherOrder: m.defaults.MatcherOrder,
		}, nil
	}

	var doc settingsDoc
	if err := json.Unmarshal(row.Settings, &doc); err != nil {
		return ingest.ProcessorSettings{}, err
	}
	return ingest.ProcessorSettings{
		LinkOrder: doc.LinkProcessors, FallbackOrder: doc.FallbackProcessors,
		MatcherOrder: doc.FuzzyMatchers,
	}, nil
}
