-- name: CreateImportJob :one
INSERT INTO import_jobs (user_id, batch_id, filename, service, total_items)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ListImportJobsForUser :many
SELECT * FROM import_jobs WHERE user_id = $1 ORDER BY created_at DESC;

-- name: GetImportJob :one
SELECT * FROM import_jobs WHERE id = $1 AND user_id = $2;

-- name: GetImportJobByID :one
SELECT * FROM import_jobs WHERE id = $1;

-- name: StartImportJob :exec
UPDATE import_jobs SET status = 'running', started_at = now() WHERE id = $1;

-- name: IncrementImportProgress :exec
UPDATE import_jobs SET
  processed_items = processed_items + 1,
  imported_items = imported_items + sqlc.arg(imported)::int,
  skipped_items = skipped_items + sqlc.arg(skipped)::int,
  failed_items = failed_items + sqlc.arg(failed)::int
WHERE id = $1;

-- name: FinishImportJob :exec
UPDATE import_jobs SET status = $2, error_message = $3, finished_at = now() WHERE id = $1;

-- name: CancelImportJob :execrows
UPDATE import_jobs SET status = 'cancelled', finished_at = now()
WHERE id = $1 AND user_id = $2 AND status IN ('queued', 'running');

-- name: ResetStaleRunningImportJobs :many
UPDATE import_jobs SET status = 'queued', started_at = NULL WHERE status = 'running' RETURNING *;
