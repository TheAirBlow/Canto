package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"Canto/internal/search"
)

// searchLimit bounds how many hits a single search request returns, per type.
const searchLimit = 25

// validSearchTypes lists every value the "type" query param accepts.
var validSearchTypes = []string{"artist", "album", "song", "user", "own_listens"}

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
	User      *userResponse      `json:"user,omitempty"`
	OwnListen *ownListenResponse `json:"own_listen,omitempty"`
}

// ownListenResponse is one of the caller's own listens, matched by its song.
type ownListenResponse struct {
	ListenID   int64        `json:"listen_id"`
	ListenedAt time.Time    `json:"listened_at"`
	Song       songResponse `json:"song"`
}

// searchQuery queries any combination of the catalog, user profiles, and the caller's own listen history.
func (s *Server) searchQuery(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		badRequest(w, "q is required")
		return
	}
	types, err := parseSearchTypes(r.URL.Query().Get("type"))
	if err != nil {
		badRequest(w, err.Error())
		return
	}

	limit := searchLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}

	var results []searchResultResponse
	for _, t := range types {
		hits, err := s.search.Search(r.Context(), t+"s", q, "", limit)
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

// parseSearchTypes splits raw on commas into a deduplicated, validated type list, defaulting to artist/album/song when empty.
func parseSearchTypes(raw string) ([]string, error) {
	if raw == "" {
		return []string{"artist", "album", "song"}, nil
	}
	seen := make(map[string]bool)
	var types []string
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		if !slices.Contains(validSearchTypes, t) {
			return nil, fmt.Errorf("type must be a comma-separated list of: %s", strings.Join(validSearchTypes, ", "))
		}
		seen[t] = true
		types = append(types, t)
	}
	if len(types) == 0 {
		return nil, fmt.Errorf("type must be a comma-separated list of: %s", strings.Join(validSearchTypes, ", "))
	}
	return types, nil
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
	case "user":
		rows, err := s.queries.GetUsersByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		out := make([]searchResultResponse, len(rows))
		for i, row := range rows {
			resp := newUserResponse(row)
			out[i] = searchResultResponse{Type: "user", User: &resp}
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
