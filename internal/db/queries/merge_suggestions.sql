-- name: QueueMergeSuggestion :exec
INSERT INTO merge_suggestions (entity_type, lo_id, hi_id, score)
VALUES (sqlc.arg(entity_type)::entity_type, LEAST(sqlc.arg(a)::bigint, sqlc.arg(b)::bigint), GREATEST(sqlc.arg(a)::bigint, sqlc.arg(b)::bigint), sqlc.arg(score)::real)
ON CONFLICT (entity_type, lo_id, hi_id) DO NOTHING;

-- name: ListMergeSuggestions :many
SELECT * FROM merge_suggestions WHERE entity_type = $1 AND NOT rejected ORDER BY score DESC;

-- name: RejectMergeSuggestion :exec
UPDATE merge_suggestions SET rejected = TRUE WHERE id = $1;

-- name: DeleteMergeSuggestionsForEntity :exec
DELETE FROM merge_suggestions WHERE entity_type = sqlc.arg(entity_type)::entity_type AND (lo_id = sqlc.arg(entity_id)::bigint OR hi_id = sqlc.arg(entity_id)::bigint);
