-- name: GetSourceEntityID :one
SELECT entity_id FROM sources WHERE entity_type = $1 AND source_type = $2 AND extracted_id = $3;

-- name: InsertSourceIfAbsent :one
INSERT INTO sources (entity_type, entity_id, source_type, raw_url, extracted_id, correlation_method, confidence)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (entity_type, source_type, extracted_id) WHERE extracted_id IS NOT NULL
DO UPDATE SET entity_id = sources.entity_id
RETURNING *;

-- name: ListSourcesForEntity :many
SELECT * FROM sources WHERE entity_type = $1 AND entity_id = $2 ORDER BY created_at;

-- name: ListEntityIDsWithSourceType :many
SELECT DISTINCT entity_id FROM sources
WHERE entity_type = sqlc.arg(entity_type)::entity_type
  AND source_type = sqlc.arg(source_type)::text
  AND entity_id = ANY(sqlc.arg(entity_ids)::bigint[]);
