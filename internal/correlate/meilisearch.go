package correlate

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"Canto/internal/search"
)

// MeilisearchMatcher matches an entity via the shared Meilisearch search client.
type MeilisearchMatcher struct {
	client *search.Client
}

// NewMeilisearchMatcher builds a MeilisearchMatcher against client.
func NewMeilisearchMatcher(client *search.Client) *MeilisearchMatcher {
	return &MeilisearchMatcher{client: client}
}

// ID identifies this matcher in configured matcher-order lists.
func (m *MeilisearchMatcher) ID() string { return "meilisearch" }

// Match searches the index for name, scoped by artistIDs/albumID when given, returning the top hit's id.
func (m *MeilisearchMatcher) Match(ctx context.Context, entityType, name string, artistIDs []int64, albumID *int64) (int64, bool, error) {
	var clauses []string
	if entityType != "artist" && len(artistIDs) > 0 {
		ids := make([]string, len(artistIDs))
		for i, id := range artistIDs {
			ids[i] = strconv.FormatInt(id, 10)
		}
		clauses = append(clauses, fmt.Sprintf("artist_ids IN [%s]", strings.Join(ids, ", ")))
	}
	if entityType == "song" && albumID != nil {
		clauses = append(clauses, fmt.Sprintf("album_id = %d", *albumID))
	}
	filter := strings.Join(clauses, " AND ")

	hits, err := m.client.Search(ctx, entityType+"s", name, filter, 1)
	if err != nil {
		return 0, false, fmt.Errorf("meilisearch: match: %w", err)
	}
	if len(hits) == 0 {
		return 0, false, nil
	}
	return hits[0].ID, true, nil
}
