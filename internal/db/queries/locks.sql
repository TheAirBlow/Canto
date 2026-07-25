-- name: AdvisoryLockKey :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(key)::text, sqlc.arg(seed)::bigint));
