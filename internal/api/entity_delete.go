package api

import (
	"context"

	"Canto/internal/db"
)

// deletePolymorphicStats removes id's rows from every table keyed by (entity_type, entity_id) without a direct foreign key, ahead of deleting the entity itself.
func deletePolymorphicStats(ctx context.Context, q *db.Queries, entityType db.EntityType, id int64) error {
	if err := q.DeleteSourcesForEntity(ctx, db.DeleteSourcesForEntityParams{EntityType: entityType, EntityID: id}); err != nil {
		return err
	}
	if err := q.DeleteAliasesForEntity(ctx, db.DeleteAliasesForEntityParams{EntityType: entityType, EntityID: id}); err != nil {
		return err
	}
	if err := q.DeleteFirstListensForEntity(ctx, db.DeleteFirstListensForEntityParams{EntityType: entityType, EntityID: id}); err != nil {
		return err
	}
	if err := q.DeleteMergeSuggestionsForEntity(ctx, db.DeleteMergeSuggestionsForEntityParams{EntityType: entityType, EntityID: id}); err != nil {
		return err
	}
	return q.DeleteGlobalStatsForEntity(ctx, db.DeleteGlobalStatsForEntityParams{EntityType: entityType, EntityID: id})
}
