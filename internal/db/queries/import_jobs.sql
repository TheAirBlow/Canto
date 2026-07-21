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

-- name: SetImportJobTotal :exec
UPDATE import_jobs SET total_items = $2 WHERE id = $1;

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
WHERE id = $1 AND user_id = $2 AND status IN ('queued', 'running', 'paused');

-- name: PauseImportJob :exec
UPDATE import_jobs SET status = 'paused' WHERE id = $1;

-- name: FailInterruptedImportJobs :many
UPDATE import_jobs SET status = 'failed',
  error_message = 'server restarted while this job was running; progress could not be verified',
  finished_at = now()
WHERE status = 'running' RETURNING *;

-- name: ListResumableImportJobs :many
SELECT * FROM import_jobs WHERE status IN ('queued', 'paused') ORDER BY created_at;
