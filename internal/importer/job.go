package importer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/db"
)

// UploadedFile is one file received in a bulk-import request.
type UploadedFile struct {
	Filename string
	Reader   io.Reader
}

// scratchDir is where uploaded files are spooled before their owning job row is committed.
func (m *Manager) scratchDir() string {
	return filepath.Join(m.dataDir, "import", "tmp")
}

// finalPath is where jobID's uploaded file lives once its row is committed.
func (m *Manager) finalPath(jobID int64, filename string) string {
	return filepath.Join(m.dataDir, "import", fmt.Sprintf("%d", jobID), filename)
}

// CreateBatch spools every file to scratch space, inserts one import_jobs row per file in one transaction, then moves each file into its job's final directory.
func (m *Manager) CreateBatch(ctx context.Context, userID int64, service ImportService, files []UploadedFile) ([]db.ImportJob, error) {
	if _, ok := m.formats[service]; !ok {
		return nil, fmt.Errorf("importer: unsupported service %q", service)
	}
	if err := os.MkdirAll(m.scratchDir(), 0o755); err != nil {
		return nil, fmt.Errorf("importer: create scratch dir: %w", err)
	}

	type spooled struct {
		filename string
		tempPath string
	}
	var spool []spooled
	renamed := false
	defer func() {
		if renamed {
			return
		}
		for _, sp := range spool {
			os.Remove(sp.tempPath)
		}
	}()

	for _, f := range files {
		tmp, err := os.CreateTemp(m.scratchDir(), "upload-*")
		if err != nil {
			return nil, fmt.Errorf("importer: create temp file: %w", err)
		}
		if _, err := io.Copy(tmp, f.Reader); err != nil {
			tmp.Close()
			return nil, fmt.Errorf("importer: spool %s: %w", f.Filename, err)
		}
		tmp.Close()
		spool = append(spool, spooled{filename: f.Filename, tempPath: tmp.Name()})
	}

	batchID := uuid.New()
	tx, err := m.dbPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := m.queries.WithTx(tx)

	jobs := make([]db.ImportJob, 0, len(spool))
	for _, sp := range spool {
		job, err := q.CreateImportJob(ctx, db.CreateImportJobParams{
			UserID: userID, BatchID: pgtype.UUID{Bytes: batchID, Valid: true},
			Filename: sp.filename, Service: string(service), TotalItems: 0,
		})
		if err != nil {
			return nil, fmt.Errorf("importer: create job row: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	for i, job := range jobs {
		finalPath := m.finalPath(job.ID, job.Filename)
		if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
			return jobs, fmt.Errorf("importer: create job dir: %w", err)
		}
		if err := os.Rename(spool[i].tempPath, finalPath); err != nil {
			return jobs, fmt.Errorf("importer: move %s into place: %w", job.Filename, err)
		}
	}
	renamed = true

	for _, job := range jobs {
		m.runAsync(job, 0)
	}
	return jobs, nil
}

// ListJobs lists userID's import jobs.
func (m *Manager) ListJobs(ctx context.Context, userID int64) ([]db.ImportJob, error) {
	return m.queries.ListImportJobsForUser(ctx, userID)
}

// GetJob returns userID's job id.
func (m *Manager) GetJob(ctx context.Context, userID, id int64) (db.ImportJob, error) {
	return m.queries.GetImportJob(ctx, db.GetImportJobParams{ID: id, UserID: userID})
}

// CancelJob cancels userID's queued, running, or paused job id, reporting whether a row was actually cancelled.
func (m *Manager) CancelJob(ctx context.Context, userID, id int64) (bool, error) {
	rows, err := m.queries.CancelImportJob(ctx, db.CancelImportJobParams{ID: id, UserID: userID})
	return rows > 0, err
}
