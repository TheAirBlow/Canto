-- name: DeleteSourcesForEntity :exec
DELETE FROM sources WHERE entity_type = sqlc.arg(entity_type)::entity_type AND entity_id = sqlc.arg(entity_id)::bigint;

-- name: DeleteAliasesForEntity :exec
DELETE FROM entity_aliases WHERE entity_type = sqlc.arg(entity_type)::entity_type AND entity_id = sqlc.arg(entity_id)::bigint;

-- name: DeleteFirstListensForEntity :exec
DELETE FROM user_entity_first_listen WHERE entity_type = sqlc.arg(entity_type)::entity_type AND entity_id = sqlc.arg(entity_id)::bigint;

-- name: DeleteGlobalStatsForEntity :exec
DELETE FROM entity_global_stats WHERE entity_type = sqlc.arg(entity_type)::entity_type AND entity_id = sqlc.arg(entity_id)::bigint;
