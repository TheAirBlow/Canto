-- name: GetSourceEntityID :one
SELECT entity_id FROM sources WHERE source_type = $1 AND extracted_id = $2;

-- name: InsertSourceIfAbsent :one
INSERT INTO sources (entity_type, entity_id, source_type, raw_url, extracted_id, correlation_method, confidence)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (source_type, extracted_id) WHERE extracted_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: ListSourcesForEntity :many
SELECT * FROM sources WHERE entity_type = $1 AND entity_id = $2 ORDER BY created_at;
