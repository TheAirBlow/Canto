package correlate

import (
	"context"
	"log/slog"

	"Canto/internal/db"
)

// Candidate is one entity a FuzzyMatcher believes might match a Query.
type Candidate struct {
	EntityID int64   // candidate entity's id
	Source   string  // matcher ID that produced this candidate
	Hint     float64 // matcher's own confidence in [0, 1], scorer input only, not the final decision
}

// Query is what a FuzzyMatcher searches candidates for.
type Query struct {
	Names       []string // name spellings to search for (raw, normalized, romanized, aliases)
	ArtistIDs   []int64  // linked artist ids, when known
	ArtistNames []string // linked artist names, when known
	AlbumID     *int64   // linked album id, songs only
	DurationMs  *int32   // declared duration, songs only
	TrackIDs    []int64  // linked song ids, albums only, when known
}

// FuzzyMatcher contributes candidate entities for a Query; the scorer, not the matcher, decides.
type FuzzyMatcher interface {
	// ID is this matcher's stable identifier, used in configured matcher-order lists.
	ID() string

	// Candidates returns every entity this matcher believes might match q, unscoped by artist/album.
	Candidates(ctx context.Context, entityType string, q Query) ([]Candidate, error)
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

// Availabler is implemented by matchers whose availability can change at runtime.
type Availabler interface {
	Available() bool
}

// ByID looks up the registered matcher for id.
func (r *MatcherRegistry) ByID(id string) (FuzzyMatcher, bool) {
	m, ok := r.matchers[id]
	return m, ok
}

// IDs returns every registered matcher's id.
func (r *MatcherRegistry) IDs() []string {
	ids := make([]string, 0, len(r.matchers))
	for id := range r.matchers {
		ids = append(ids, id)
	}
	return ids
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

// collectCandidates runs every matcher and unions their candidates by entity id, keeping each entity's highest hint.
func collectCandidates(ctx context.Context, matchers []FuzzyMatcher, entityType string, q Query) ([]Candidate, error) {
	byID := make(map[int64]Candidate)
	for _, m := range matchers {
		candidates, err := m.Candidates(ctx, entityType, q)
		if err != nil {
			return nil, err
		}
		for _, c := range candidates {
			if existing, ok := byID[c.EntityID]; !ok || c.Hint > existing.Hint {
				byID[c.EntityID] = c
			}
		}
	}
	out := make([]Candidate, 0, len(byID))
	for _, c := range byID {
		out = append(out, c)
	}
	return out, nil
}

// ExactMatcher matches entities whose raw name is exactly equal to one of the query's name spellings.
type ExactMatcher struct {
	queries *db.Queries
}

// NewExactMatcher builds an ExactMatcher backed by queries.
func NewExactMatcher(queries *db.Queries) *ExactMatcher {
	return &ExactMatcher{queries: queries}
}

// ID identifies this matcher in configured matcher-order lists.
func (m *ExactMatcher) ID() string { return "exact" }

// Candidates looks up every entity with a name exactly equal to one of q.Names.
func (m *ExactMatcher) Candidates(ctx context.Context, entityType string, q Query) ([]Candidate, error) {
	seen := make(map[int64]bool)
	var out []Candidate
	add := func(id int64) {
		if !seen[id] {
			seen[id] = true
			out = append(out, Candidate{EntityID: id, Source: m.ID(), Hint: 1})
		}
	}

	for _, name := range q.Names {
		if name == "" {
			continue
		}
		ids, err := m.exactIDs(ctx, entityType, name, q.ArtistIDs, q.AlbumID)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			add(id)
		}
	}
	return out, nil
}

// exactIDs looks up every entity id with an exact raw-name match, scoped by artistIDs/albumID when given.
func (m *ExactMatcher) exactIDs(ctx context.Context, entityType, name string, artistIDs []int64, albumID *int64) ([]int64, error) {
	switch entityType {
	case "artist":
		rows, err := m.queries.FindArtistsByExactName(ctx, name)
		return artistIDsOf(rows), err
	case "album":
		if len(artistIDs) == 0 {
			rows, err := m.queries.FindAlbumsByExactName(ctx, name)
			return albumIDsOf(rows), err
		}
		rows, err := m.queries.FindAlbumsByExactNameForArtists(ctx, db.FindAlbumsByExactNameForArtistsParams{Name: name, ArtistIds: artistIDs})
		return albumIDsOf(rows), err
	case "song":
		switch {
		case len(artistIDs) == 0:
			rows, err := m.queries.FindSongsByExactName(ctx, name)
			return songIDsOf(rows), err
		case albumID == nil:
			rows, err := m.queries.FindSongsByExactNameForArtists(ctx, db.FindSongsByExactNameForArtistsParams{Name: name, ArtistIds: artistIDs})
			return songIDsOf(rows), err
		default:
			rows, err := m.queries.FindSongsByExactNameForArtistsAndAlbum(ctx, db.FindSongsByExactNameForArtistsAndAlbumParams{Name: name, ArtistIds: artistIDs, AlbumID: *albumID})
			return songIDsOf(rows), err
		}
	default:
		return nil, nil
	}
}

func artistIDsOf(rows []db.Artist) []int64 {
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

func albumIDsOf(rows []db.Album) []int64 {
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

func songIDsOf(rows []db.Song) []int64 {
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// matchResult turns a single-row query into a found/not-found result, used by lock-recheck call sites.
func matchResult(id int64, err error) (int64, bool, error) {
	if isNoRows(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// exactRecheck looks up a single exact raw-name match under q's transaction, scoped by artistIDs/albumID when given, for the advisory-lock recheck.
func exactRecheck(ctx context.Context, q *db.Queries, entityType, name string, artistIDs []int64, albumID *int64) (int64, bool, error) {
	switch entityType {
	case "artist":
		row, err := q.FindArtistByExactName(ctx, name)
		return matchResult(row.ID, err)
	case "album":
		if len(artistIDs) == 0 {
			row, err := q.FindAlbumByExactName(ctx, name)
			return matchResult(row.ID, err)
		}
		row, err := q.FindAlbumByExactNameForArtists(ctx, db.FindAlbumByExactNameForArtistsParams{Name: name, ArtistIds: artistIDs})
		return matchResult(row.ID, err)
	case "song":
		switch {
		case len(artistIDs) == 0:
			row, err := q.FindSongByExactName(ctx, name)
			return matchResult(row.ID, err)
		case albumID == nil:
			row, err := q.FindSongByExactNameForArtists(ctx, db.FindSongByExactNameForArtistsParams{Name: name, ArtistIds: artistIDs})
			return matchResult(row.ID, err)
		default:
			row, err := q.FindSongByExactNameForArtistsAndAlbum(ctx, db.FindSongByExactNameForArtistsAndAlbumParams{Name: name, ArtistIds: artistIDs, AlbumID: *albumID})
			return matchResult(row.ID, err)
		}
	default:
		return 0, false, nil
	}
}
