-- name: AdvisoryLockEntityName :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(name_normalized)::text, sqlc.arg(seed)::bigint));
