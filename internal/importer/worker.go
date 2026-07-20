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
)

// runTimeout bounds one entire import job, independent of the request that triggered it.
const runTimeout = 6 * time.Hour

// runAsync starts job's processing in the background.
func (m *Manager) runAsync(job db.ImportJob) {
	go m.runJob(job)
}

// runJob parses job's file and submits every entry through the shared worker pool, updating progress as it goes.
func (m *Manager) runJob(job db.ImportJob) {
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	slog.Info("importer: job started", "id", job.ID, "service", job.Service, "filename", job.Filename)
	if err := m.queries.StartImportJob(ctx, job.ID); err != nil {
		slog.Error("importer: mark job running failed", "id", job.ID, "err", err)
		return
	}

	format, ok := m.formats[job.Service]
	if !ok {
		m.finishJob(ctx, job.ID, db.ImportStatusFailed, "unsupported format")
		return
	}
	file, err := os.Open(m.finalPath(job.ID, job.Filename))
	if err != nil {
		m.finishJob(ctx, job.ID, db.ImportStatusFailed, err.Error())
		return
	}
	defer file.Close()

	settings, err := m.resolveSettings(ctx, job.UserID)
	if err != nil {
		m.finishJob(ctx, job.ID, db.ImportStatusFailed, err.Error())
		return
	}

	var wg sync.WaitGroup
	parseErr := format.Parse(file, func(in ingest.ListenInput) {
		in.UserID = job.UserID
		wg.Add(1)
		m.sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-m.sem }()
			m.processEntry(ctx, job.ID, in, settings)
		}()
	})
	wg.Wait()

	if parseErr != nil {
		m.finishJob(ctx, job.ID, db.ImportStatusFailed, parseErr.Error())
		return
	}
	m.finishJob(ctx, job.ID, db.ImportStatusCompleted, "")
}

// processEntry submits one parsed listen and records the outcome in job's progress counters.
func (m *Manager) processEntry(ctx context.Context, jobID int64, in ingest.ListenInput, settings ingest.ProcessorSettings) {
	progress := db.IncrementImportProgressParams{ID: jobID}

	switch {
	case in.SongName == "" && in.OriginURL == "":
		progress.Skipped = 1
	default:
		if _, err := m.ingest.SubmitListen(ctx, in, settings, false); err != nil {
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
	FuzzyNormalize     bool     `json:"fuzzy_normalize"`
}

// resolveSettings loads userID's stored processor settings, falling back to configured defaults.
func (m *Manager) resolveSettings(ctx context.Context, userID int64) (ingest.ProcessorSettings, error) {
	row, err := m.queries.GetUserSettings(ctx, userID)
	if err != nil {
		return ingest.ProcessorSettings{
			LinkOrder: m.defaults.LinkOrder, FallbackOrder: m.defaults.FallbackOrder,
			MatcherOrder: m.defaults.MatcherOrder, Normalize: m.defaults.Normalize,
		}, nil
	}

	var doc settingsDoc
	if err := json.Unmarshal(row.Settings, &doc); err != nil {
		return ingest.ProcessorSettings{}, err
	}
	return ingest.ProcessorSettings{
		LinkOrder: doc.LinkProcessors, FallbackOrder: doc.FallbackProcessors,
		MatcherOrder: doc.FuzzyMatchers, Normalize: doc.FuzzyNormalize,
	}, nil
}
