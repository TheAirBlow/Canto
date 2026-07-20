-- name: CreateInvite :one
INSERT INTO invites (code, name, max_uses, created_by) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: ListInvites :many
SELECT * FROM invites ORDER BY created_at DESC;

-- name: DeleteInvite :execrows
DELETE FROM invites WHERE id = $1;

-- name: ClaimInvite :one
UPDATE invites SET uses_count = uses_count + 1
WHERE code = $1 AND (max_uses IS NULL OR uses_count < max_uses)
RETURNING *;
