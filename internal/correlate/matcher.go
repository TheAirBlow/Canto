package correlate

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"Canto/internal/db"
)

// FuzzyMatcher resolves an entity by approximate name when no exact source match exists.
type FuzzyMatcher interface {
	// ID is this matcher's stable identifier, used in configured matcher-order lists.
	ID() string

	// Match returns the best-matching existing entity id for name, scoped by artistIDs and albumID when given.
	Match(ctx context.Context, entityType string, name string, artistIDs []int64, albumID *int64) (entityID int64, found bool, err error)
}

// MatcherRegistry holds every known FuzzyMatcher, keyed by ID.
type MatcherRegistry struct {
	matchers map[string]FuzzyMatcher
}

// NewMatcherRegistry builds a MatcherRegistry from matchers.
func NewMatcherRegistry(matchers ...FuzzyMatcher) *MatcherRegistry {
	m := make(map[string]FuzzyMatcher, len(matchers))
	for _, matcher := range matchers {
		m[matcher.ID()] = matcher
	}
	return &MatcherRegistry{matchers: m}
}

// Ordered resolves ids to registered matchers, in order, warning and skipping any id that's unknown.
func (r *MatcherRegistry) Ordered(ids []string) []FuzzyMatcher {
	out := make([]FuzzyMatcher, 0, len(ids))
	for _, id := range ids {
		m, ok := r.matchers[id]
		if !ok {
			slog.Warn("configured fuzzy matcher not registered, skipping", "id", id)
			continue
		}
		out = append(out, m)
	}
	return out
}

// chainMatch tries every matcher in order, returning the first match found.
func chainMatch(ctx context.Context, matchers []FuzzyMatcher, entityType, name string, artistIDs []int64, albumID *int64) (int64, bool, error) {
	for _, m := range matchers {
		id, found, err := m.Match(ctx, entityType, name, artistIDs, albumID)
		if err != nil {
			return 0, false, err
		}
		if found {
			return id, true, nil
		}
	}
	return 0, false, nil
}

// ExactMatcher matches an entity whose name_normalized is exactly equal to the query.
type ExactMatcher struct {
	queries *db.Queries
}

// NewExactMatcher builds an ExactMatcher backed by queries.
func NewExactMatcher(queries *db.Queries) *ExactMatcher {
	return &ExactMatcher{queries: queries}
}

// ID identifies this matcher in configured matcher-order lists.
func (m *ExactMatcher) ID() string { return "exact" }

// Match looks up an exact name_normalized match, scoped by artistIDs/albumID when given.
func (m *ExactMatcher) Match(ctx context.Context, entityType, name string, artistIDs []int64, albumID *int64) (int64, bool, error) {
	switch entityType {
	case "artist":
		row, err := m.queries.FindArtistByExactName(ctx, name)
		return matchResult(row.ID, err)
	case "album":
		if len(artistIDs) == 0 {
			row, err := m.queries.FindAlbumByExactName(ctx, name)
			return matchResult(row.ID, err)
		}
		row, err := m.queries.FindAlbumByExactNameForArtists(ctx, db.FindAlbumByExactNameForArtistsParams{Name: name, ArtistIds: artistIDs})
		return matchResult(row.ID, err)
	case "song":
		switch {
		case len(artistIDs) == 0:
			row, err := m.queries.FindSongByExactName(ctx, name)
			return matchResult(row.ID, err)
		case albumID == nil:
			row, err := m.queries.FindSongByExactNameForArtists(ctx, db.FindSongByExactNameForArtistsParams{Name: name, ArtistIds: artistIDs})
			return matchResult(row.ID, err)
		default:
			row, err := m.queries.FindSongByExactNameForArtistsAndAlbum(ctx, db.FindSongByExactNameForArtistsAndAlbumParams{Name: name, ArtistIds: artistIDs, AlbumID: *albumID})
			return matchResult(row.ID, err)
		}
	default:
		return 0, false, nil
	}
}

// matchResult turns a single-row query into the FuzzyMatcher.Match result shape.
func matchResult(id int64, err error) (int64, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}
