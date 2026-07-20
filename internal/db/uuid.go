package db

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UUID converts a nullable pgtype.UUID into a uuid.UUID, reporting whether it was set.
func UUID(id pgtype.UUID) (uuid.UUID, bool) {
	if !id.Valid {
		return uuid.UUID{}, false
	}
	return uuid.UUID(id.Bytes), true
}
