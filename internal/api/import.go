package api

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"Canto/internal/auth"
	"Canto/internal/db"
	"Canto/internal/importer"
)

// maxImportUploadBytes bounds one bulk-import request's total multipart body size.
const maxImportUploadBytes = 1 << 30 // 1GiB

// registerImport registers the bulk-import endpoints.
func (s *Server) registerImport(mux authMux) {
	mux.CookieAuthHandleFunc("POST /import", s.createImportBatch)
	mux.CookieAuthHandleFunc("GET /import", s.listImportJobs)
	mux.CookieAuthHandleFunc("GET /import/{id}", s.getImportJob)
	mux.CookieAuthHandleFunc("DELETE /import/{id}", s.cancelImportJob)
}

// importJobResponse is the public-facing import job shape.
type importJobResponse struct {
	ID             int64      `json:"id"`
	BatchID        string     `json:"batch_id"`
	Filename       string     `json:"filename"`
	Service        string     `json:"service"`
	Status         string     `json:"status"`
	TotalItems     int32      `json:"total_items"`
	ProcessedItems int32      `json:"processed_items"`
	ImportedItems  int32      `json:"imported_items"`
	SkippedItems   int32      `json:"skipped_items"`
	FailedItems    int32      `json:"failed_items"`
	ErrorMessage   *string    `json:"error_message,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

// newImportJobResponse builds an importJobResponse from a db.ImportJob.
func newImportJobResponse(j db.ImportJob) importJobResponse {
	var batchID string
	if j.BatchID.Valid {
		batchID = uuid.UUID(j.BatchID.Bytes).String()
	}
	resp := importJobResponse{
		ID: j.ID, BatchID: batchID, Filename: j.Filename, Service: string(j.Service), Status: string(j.Status),
		TotalItems: j.TotalItems, ProcessedItems: j.ProcessedItems, ImportedItems: j.ImportedItems,
		SkippedItems: j.SkippedItems, FailedItems: j.FailedItems, ErrorMessage: j.ErrorMessage, CreatedAt: j.CreatedAt.Time,
	}
	if j.StartedAt.Valid {
		resp.StartedAt = &j.StartedAt.Time
	}
	if j.FinishedAt.Valid {
		resp.FinishedAt = &j.FinishedAt.Time
	}
	return resp
}

// createImportBatch accepts one or more files under a single service for the caller to bulk-import.
func (s *Server) createImportBatch(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, maxImportUploadBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		badRequest(w, err.Error())
		return
	}

	service := r.FormValue("service")
	if service == "" {
		badRequest(w, "service is required")
		return
	}
	fileHeaders := r.MultipartForm.File["files[]"]
	if len(fileHeaders) == 0 {
		badRequest(w, "at least one files[] part is required")
		return
	}

	files := make([]importer.UploadedFile, 0, len(fileHeaders))
	for _, fh := range fileHeaders {
		f, err := fh.Open()
		if err != nil {
			badRequest(w, err.Error())
			return
		}
		defer f.Close()
		files = append(files, importer.UploadedFile{Filename: fh.Filename, Reader: f})
	}

	jobs, err := s.importer.CreateBatch(r.Context(), user.ID, db.ImportService(service), files)
	if err != nil {
		badRequest(w, err.Error())
		return
	}

	resp := make([]importJobResponse, len(jobs))
	for i, job := range jobs {
		resp[i] = newImportJobResponse(job)
	}
	created(w, map[string]any{"jobs": resp})
}

// listImportJobs lists the caller's import jobs.
func (s *Server) listImportJobs(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	jobs, err := s.importer.ListJobs(r.Context(), user.ID)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	resp := make([]importJobResponse, len(jobs))
	for i, job := range jobs {
		resp[i] = newImportJobResponse(job)
	}
	ok(w, resp)
}

// getImportJob returns one of the caller's import jobs by id.
func (s *Server) getImportJob(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	job, err := s.importer.GetJob(r.Context(), user.ID, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ok(w, newImportJobResponse(job))
}

// cancelImportJob cancels one of the caller's queued or running import jobs.
func (s *Server) cancelImportJob(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	cancelled, err := s.importer.CancelJob(r.Context(), user.ID, id)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	if !cancelled {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
