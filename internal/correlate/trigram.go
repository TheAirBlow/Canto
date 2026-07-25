package correlate

import (
	"context"
	"fmt"

	"Canto/internal/correlate/romanize"
	"Canto/internal/db"
)

// TrigramMatcher matches candidates via Postgres pg_trgm similarity on name_normalized/name_romanized.
type TrigramMatcher struct {
	queries *db.Queries
	limit   int32
}

// NewTrigramMatcher builds a TrigramMatcher backed by queries, retrieving up to limit hits per column per query name.
func NewTrigramMatcher(queries *db.Queries, limit int32) *TrigramMatcher {
	return &TrigramMatcher{queries: queries, limit: limit}
}

// ID identifies this matcher in configured matcher-order lists.
func (m *TrigramMatcher) ID() string { return "trigram" }

// Candidates runs a pg_trgm similarity search against name_normalized and name_romanized for every name in q.Names, unfiltered by artist/album.
func (m *TrigramMatcher) Candidates(ctx context.Context, entityType string, q Query) ([]Candidate, error) {
	byID := make(map[int64]float64)
	fold := func(id int64, sim float64) {
		if sim > byID[id] {
			byID[id] = sim
		}
	}

	for _, name := range q.Names {
		if name == "" {
			continue
		}
		norm := NormalizeName(name)
		roman := romanize.Romanize(name)

		switch entityType {
		case "artist":
			rows, err := m.queries.TrigramMatchArtistsByNormalized(ctx, db.TrigramMatchArtistsByNormalizedParams{Query: norm, MaxRows: m.limit})
			if err != nil {
				return nil, fmt.Errorf("correlate: trigram: match artists by normalized: %w", err)
			}
			for _, r := range rows {
				fold(r.ID, r.Sim)
			}
			if roman != "" {
				rows, err := m.queries.TrigramMatchArtistsByRomanized(ctx, db.TrigramMatchArtistsByRomanizedParams{Query: roman, MaxRows: m.limit})
				if err != nil {
					return nil, fmt.Errorf("correlate: trigram: match artists by romanized: %w", err)
				}
				for _, r := range rows {
					fold(r.ID, r.Sim)
				}
			}

		case "album":
			rows, err := m.queries.TrigramMatchAlbumsByNormalized(ctx, db.TrigramMatchAlbumsByNormalizedParams{Query: norm, MaxRows: m.limit})
			if err != nil {
				return nil, fmt.Errorf("correlate: trigram: match albums by normalized: %w", err)
			}
			for _, r := range rows {
				fold(r.ID, r.Sim)
			}
			if roman != "" {
				rows, err := m.queries.TrigramMatchAlbumsByRomanized(ctx, db.TrigramMatchAlbumsByRomanizedParams{Query: roman, MaxRows: m.limit})
				if err != nil {
					return nil, fmt.Errorf("correlate: trigram: match albums by romanized: %w", err)
				}
				for _, r := range rows {
					fold(r.ID, r.Sim)
				}
			}

		case "song":
			rows, err := m.queries.TrigramMatchSongsByNormalized(ctx, db.TrigramMatchSongsByNormalizedParams{Query: norm, MaxRows: m.limit})
			if err != nil {
				return nil, fmt.Errorf("correlate: trigram: match songs by normalized: %w", err)
			}
			for _, r := range rows {
				fold(r.ID, r.Sim)
			}
			if roman != "" {
				rows, err := m.queries.TrigramMatchSongsByRomanized(ctx, db.TrigramMatchSongsByRomanizedParams{Query: roman, MaxRows: m.limit})
				if err != nil {
					return nil, fmt.Errorf("correlate: trigram: match songs by romanized: %w", err)
				}
				for _, r := range rows {
					fold(r.ID, r.Sim)
				}
			}
		}
	}

	out := make([]Candidate, 0, len(byID))
	for id, sim := range byID {
		out = append(out, Candidate{EntityID: id, Source: m.ID(), Hint: sim})
	}
	return out, nil
}
