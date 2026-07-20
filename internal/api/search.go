package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"Canto/internal/db"
	"Canto/internal/search"
)

// searchLimit bounds how many hits a single search request returns.
const searchLimit = 25

// registerSearch registers the search endpoint.
func (s *Server) registerSearch(mux authMux) {
	mux.HandleFunc("GET /search", s.searchQuery)
}

// searchResultResponse is one search hit.
type searchResultResponse struct {
	Type      string             `json:"type"`
	Artist    *artistResponse    `json:"artist,omitempty"`
	Album     *albumResponse     `json:"album,omitempty"`
	Song      *songResponse      `json:"song,omitempty"`
	OwnListen *ownListenResponse `json:"own_listen,omitempty"`
}

// ownListenResponse is one of the caller's own listens, matched by its song.
type ownListenResponse struct {
	ListenID   int64        `json:"listen_id"`
	ListenedAt time.Time    `json:"listened_at"`
	Song       songResponse `json:"song"`
}

// searchQuery queries the catalog or the caller's own listen history.
func (s *Server) searchQuery(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		badRequest(w, "q is required")
		return
	}
	entityType := r.URL.Query().Get("type")
	if entityType == "" {
		entityType = "all"
	}

	if entityType == "own_listens" {
		s.searchOwnListens(w, r, q)
		return
	}

	var types []string
	switch entityType {
	case "artist", "album", "song":
		types = []string{entityType}
	case "all":
		types = []string{"artist", "album", "song"}
	default:
		badRequest(w, "type must be artist, album, song, all, or own_listens")
		return
	}

	var results []searchResultResponse
	for _, t := range types {
		hits, err := s.search.Search(r.Context(), t+"s", q, "", searchLimit)
		if err != nil {
			internalError(w, err.Error())
			return
		}
		rows, err := s.searchResults(r.Context(), t, hits)
		if err != nil {
			internalError(w, err.Error())
			return
		}
		results = append(results, rows...)
	}
	ok(w, results)
}

// searchResults batch-fetches full entity rows for hits of entityType.
func (s *Server) searchResults(ctx context.Context, entityType string, hits []search.Hit) ([]searchResultResponse, error) {
	if len(hits) == 0 {
		return nil, nil
	}
	ids := make([]int64, len(hits))
	for i, hit := range hits {
		ids[i] = hit.ID
	}

	switch entityType {
	case "artist":
		rows, err := s.queries.GetArtistsByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		out := make([]searchResultResponse, len(rows))
		for i, row := range rows {
			resp := newArtistResponse(row)
			out[i] = searchResultResponse{Type: "artist", Artist: &resp}
		}
		return out, nil
	case "album":
		rows, err := s.queries.GetAlbumsByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		out := make([]searchResultResponse, len(rows))
		for i, row := range rows {
			resp := newAlbumResponse(row)
			out[i] = searchResultResponse{Type: "album", Album: &resp}
		}
		return out, nil
	default:
		rows, err := s.queries.GetSongsByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		out := make([]searchResultResponse, len(rows))
		for i, row := range rows {
			resp := newSongResponse(row)
			out[i] = searchResultResponse{Type: "song", Song: &resp}
		}
		return out, nil
	}
}

// searchOwnListens searches the caller's own listen history by song/artist name.
func (s *Server) searchOwnListens(w http.ResponseWriter, r *http.Request, q string) {
	user, err := s.authenticateCookie(r)
	if err != nil {
		if isAuthError(err) {
			unauthorized(w, err.Error())
		} else {
			internalError(w, err.Error())
		}
		return
	}

	limit := searchLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}

	rows, err := s.queries.SearchListensForUser(r.Context(), db.SearchListensForUserParams{
		UserID: user.ID, Query: q, MaxRows: int32(limit),
	})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	if len(rows) == 0 {
		ok(w, []searchResultResponse{})
		return
	}

	songIDs := make([]int64, len(rows))
	for i, row := range rows {
		songIDs[i] = row.SongID
	}
	songs, err := s.queries.GetSongsByIDs(r.Context(), songIDs)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	songByID := make(map[int64]db.Song, len(songs))
	for _, song := range songs {
		songByID[song.ID] = song
	}

	results := make([]searchResultResponse, 0, len(rows))
	for _, row := range rows {
		song, ok := songByID[row.SongID]
		if !ok {
			continue
		}
		listen := ownListenResponse{ListenID: row.ID, ListenedAt: row.ListenedAt.Time, Song: newSongResponse(song)}
		results = append(results, searchResultResponse{Type: "own_listen", OwnListen: &listen})
	}
	ok(w, results)
}
