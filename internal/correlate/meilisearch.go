package correlate

import (
	"context"
	"fmt"

	"Canto/internal/search"
)

// MeilisearchMatcher matches candidates via the shared Meilisearch search client.
type MeilisearchMatcher struct {
	client *search.Client
	limit  int
}

// NewMeilisearchMatcher builds a MeilisearchMatcher against client, retrieving up to limit hits per query name.
func NewMeilisearchMatcher(client *search.Client, limit int) *MeilisearchMatcher {
	return &MeilisearchMatcher{client: client, limit: limit}
}

// ID identifies this matcher in configured matcher-order lists.
func (m *MeilisearchMatcher) ID() string { return "meilisearch" }

// Available reports whether the backing Meilisearch client is configured.
func (m *MeilisearchMatcher) Available() bool { return m.client.Enabled() }

// Candidates searches the index for every name in q.Names, unfiltered by artist/album.
func (m *MeilisearchMatcher) Candidates(ctx context.Context, entityType string, q Query) ([]Candidate, error) {
	seen := make(map[int64]bool)
	var out []Candidate
	for _, name := range q.Names {
		if name == "" {
			continue
		}
		hits, err := m.client.Search(ctx, entityType+"s", name, "", m.limit)
		if err != nil {
			return nil, fmt.Errorf("meilisearch: candidates: %w", err)
		}
		for _, hit := range hits {
			if seen[hit.ID] {
				continue
			}
			seen[hit.ID] = true
			out = append(out, Candidate{EntityID: hit.ID, Source: m.ID(), Hint: hit.RankingScore})
		}
	}
	return out, nil
}
