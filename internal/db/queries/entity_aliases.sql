-- name: CreateAlias :one
INSERT INTO entity_aliases (entity_type, entity_id, alias) VALUES ($1, $2, $3)
ON CONFLICT (entity_type, entity_id, alias) DO NOTHING
RETURNING *;

-- name: ListAliasesForEntity :many
SELECT * FROM entity_aliases WHERE entity_type = $1 AND entity_id = $2 ORDER BY alias;

-- name: DeleteAlias :execrows
DELETE FROM entity_aliases WHERE id = $1 AND entity_type = $2 AND entity_id = $3;

-- name: RepointAliases :exec
UPDATE entity_aliases SET entity_id = sqlc.arg(new_entity_id)::bigint
WHERE entity_type = sqlc.arg(entity_type) AND entity_id = sqlc.arg(old_entity_id)::bigint;

-- name: SearchAliases :many
SELECT * FROM entity_aliases WHERE entity_type = $1 AND alias ILIKE '%' || sqlc.arg(query)::text || '%' LIMIT sqlc.arg(max_rows)::int;
