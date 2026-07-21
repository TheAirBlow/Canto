package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/auth"
	"Canto/internal/db"
	"Canto/internal/stats"
)

// statsPageDefault/statsPageMax bound every paginated stats/listens endpoint.
const (
	statsPageDefault = 20
	statsPageMax     = 100
)

// registerStats registers the statistics and raw-listen-history endpoints.
func (s *Server) registerStats(mux authMux) {
	mux.CookieAuthHandleFunc("GET /stats/summary", s.statsSummary)
	mux.CookieAuthHandleFunc("GET /stats/top/{kind}", s.statsTop)
	mux.CookieAuthHandleFunc("GET /stats/activity", s.statsActivity)
	mux.CookieAuthHandleFunc("GET /stats/interest/{type}/{id}", s.statsInterest)
	mux.CookieAuthHandleFunc("GET /stats/clock", s.statsClock)
	mux.CookieAuthHandleFunc("GET /stats/sources", s.statsSources)
	mux.CookieAuthHandleFunc("GET /stats/discovery", s.statsDiscovery)
	mux.CookieAuthHandleFunc("GET /stats/rewind", s.statsRewind)
	mux.CookieAuthHandleFunc("GET /stats/now-playing", s.statsNowPlaying)
	mux.CookieAuthHandleFunc("GET /listens", s.listListens)
}

// statsSummary returns the caller's overall listening summary.
func (s *Server) statsSummary(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	tf, err := parseTimeframe(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	data, err := s.stats.Summary(r.Context(), scopedUserID(r, user.ID), tf)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsTop returns the caller's artist/album/track leaderboard.
func (s *Server) statsTop(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	var kind stats.TopKind
	switch r.PathValue("kind") {
	case "artists":
		kind = stats.TopArtists
	case "albums":
		kind = stats.TopAlbums
	case "tracks":
		kind = stats.TopTracks
	default:
		badRequest(w, "kind must be artists, albums, or tracks")
		return
	}

	tf, err := parseTimeframe(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	artistID, err := parseOptionalID(r, "artist_id")
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	albumID, err := parseOptionalID(r, "album_id")
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	if kind != stats.TopTracks && (artistID != nil || albumID != nil) {
		badRequest(w, "artist_id/album_id scoping only applies to tracks")
		return
	}
	page, perPage := parsePagination(r)

	data, err := s.stats.Top(r.Context(), scopedUserID(r, user.ID), kind, tf, artistID, albumID, page, perPage)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsActivity returns the caller's zero-filled listen-count time series.
func (s *Server) statsActivity(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	tf, err := parseTimeframe(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	step := r.URL.Query().Get("step")
	if step == "" {
		step = "day"
	}
	entityType, entityID, err := parseEntityScope(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}

	data, err := s.stats.Activity(r.Context(), scopedUserID(r, user.ID), tf, step, entityType, entityID)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsInterest returns the caller's decay/growth-of-interest graph for one catalog entity.
func (s *Server) statsInterest(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	entityType := r.PathValue("type")
	switch entityType {
	case "artist", "album", "song":
	default:
		badRequest(w, "type must be artist, album, or song")
		return
	}
	entityID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid id")
		return
	}

	data, err := s.stats.Interest(r.Context(), scopedUserID(r, user.ID), entityType, entityID)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsClock returns the caller's hour x weekday listening-clock heatmap.
func (s *Server) statsClock(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	tf, err := parseTimeframe(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	data, err := s.stats.Clock(r.Context(), scopedUserID(r, user.ID), tf)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsSources returns the caller's listen count broken down by source_type.
func (s *Server) statsSources(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	tf, err := parseTimeframe(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	data, err := s.stats.Sources(r.Context(), scopedUserID(r, user.ID), tf)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsDiscovery returns the caller's new-vs-repeat listen trend.
func (s *Server) statsDiscovery(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	tf, err := parseTimeframe(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	step := r.URL.Query().Get("step")
	if step == "" {
		step = "day"
	}
	data, err := s.stats.Discovery(r.Context(), scopedUserID(r, user.ID), tf, step)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsRewind returns the caller's Spotify-Wrapped-style bundle for a period.
func (s *Server) statsRewind(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	tf, err := parseTimeframe(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	data, err := s.stats.Rewind(r.Context(), user.ID, tf)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// nowPlayingResponse is the caller's current now-playing state.
type nowPlayingResponse struct {
	Song       songResponse `json:"song"`
	StartedAt  time.Time    `json:"started_at"`
	DurationMs *int32       `json:"duration_ms,omitempty"`
}

// statsNowPlaying returns the caller's current now-playing track, live (never cached).
func (s *Server) statsNowPlaying(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	row, err := s.queries.GetNowPlaying(r.Context(), user.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		ok(w, nil)
		return
	}
	if err != nil {
		internalError(w, err.Error())
		return
	}
	song, err := s.queries.GetSongByID(r.Context(), row.SongID)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, nowPlayingResponse{Song: newSongResponse(song), StartedAt: row.StartedAt.Time, DurationMs: row.DurationMs})
}

// statsListenResponse is one raw listen in the caller's history.
type statsListenResponse struct {
	ID         int64        `json:"id"`
	Song       songResponse `json:"song"`
	ListenedAt time.Time    `json:"listened_at"`
}

// listListensResponse is GET /listens's paginated payload.
type listListensResponse struct {
	Listens []statsListenResponse `json:"listens"`
	Total   int64                 `json:"total"`
	Page    int                   `json:"page"`
	PerPage int                   `json:"per_page"`
}

// listListens returns the caller's paginated raw listen history, filterable by timeframe/track/album/artist/source.
func (s *Server) listListens(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	tf, err := parseTimeframe(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	from, to, err := tf.Resolve(time.Now())
	if err != nil {
		badRequest(w, err.Error())
		return
	}

	songID, err := parseOptionalID(r, "song_id")
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	albumID, err := parseOptionalID(r, "album_id")
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	artistID, err := parseOptionalID(r, "artist_id")
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	var sourceType *string
	if v := r.URL.Query().Get("source_type"); v != "" {
		sourceType = &v
	}
	page, perPage := parsePagination(r)
	offset := int32((page - 1) * perPage)

	params := db.ListListensForUserFilteredParams{
		UserID: user.ID, FromTime: pgtype.Timestamptz{Time: from, Valid: true}, ToTime: pgtype.Timestamptz{Time: to, Valid: true},
		SongID: songID, AlbumID: albumID, ArtistID: artistID, SourceType: sourceType, MaxRows: int32(perPage), RowOffset: offset,
	}
	rows, err := s.queries.ListListensForUserFiltered(r.Context(), params)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	total, err := s.queries.CountListensForUserFiltered(r.Context(), db.CountListensForUserFilteredParams{
		UserID: user.ID, FromTime: params.FromTime, ToTime: params.ToTime,
		SongID: songID, AlbumID: albumID, ArtistID: artistID, SourceType: sourceType,
	})
	if err != nil {
		internalError(w, err.Error())
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

	listens := make([]statsListenResponse, 0, len(rows))
	for _, row := range rows {
		song, ok := songByID[row.SongID]
		if !ok {
			continue
		}
		listens = append(listens, statsListenResponse{ID: row.ID, Song: newSongResponse(song), ListenedAt: row.ListenedAt.Time})
	}
	ok(w, listListensResponse{Listens: listens, Total: total, Page: page, PerPage: perPage})
}

// scopedUserID returns nil (every user) for scope=global, else userID.
func scopedUserID(r *http.Request, userID int64) *int64 {
	if r.URL.Query().Get("scope") == "global" {
		return nil
	}
	return &userID
}

// parseTimeframe reads the shared period/from/to/year/month/week/tz query params into a stats.Timeframe.
func parseTimeframe(r *http.Request) (stats.Timeframe, error) {
	q := r.URL.Query()
	tf := stats.Timeframe{Period: q.Get("period"), TZ: q.Get("tz")}

	if v := q.Get("from"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return tf, fmt.Errorf("invalid from")
		}
		tf.From = &n
	}
	if v := q.Get("to"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return tf, fmt.Errorf("invalid to")
		}
		tf.To = &n
	}
	if v := q.Get("year"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return tf, fmt.Errorf("invalid year")
		}
		tf.Year = &n
	}
	if v := q.Get("month"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return tf, fmt.Errorf("invalid month")
		}
		tf.Month = &n
	}
	if v := q.Get("week"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return tf, fmt.Errorf("invalid week")
		}
		tf.Week = &n
	}
	return tf, nil
}

// parsePagination reads page/per_page query params, defaulting and capping per_page.
func parsePagination(r *http.Request) (page, perPage int) {
	page, perPage = 1, statsPageDefault
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perPage = n
		}
	}
	if perPage > statsPageMax {
		perPage = statsPageMax
	}
	return page, perPage
}

// parseOptionalID reads name as an optional int64 query param.
func parseOptionalID(r *http.Request, name string) (*int64, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid %s", name)
	}
	return &id, nil
}

// parseEntityScope reads at most one of artist_id/album_id/song_id off the request.
func parseEntityScope(r *http.Request) (entityType string, entityID *int64, err error) {
	artistID, err := parseOptionalID(r, "artist_id")
	if err != nil {
		return "", nil, err
	}
	albumID, err := parseOptionalID(r, "album_id")
	if err != nil {
		return "", nil, err
	}
	songID, err := parseOptionalID(r, "song_id")
	if err != nil {
		return "", nil, err
	}

	set := 0
	for _, id := range []*int64{artistID, albumID, songID} {
		if id != nil {
			set++
		}
	}
	switch {
	case set > 1:
		return "", nil, fmt.Errorf("at most one of artist_id/album_id/song_id")
	case artistID != nil:
		return "artist", artistID, nil
	case albumID != nil:
		return "album", albumID, nil
	case songID != nil:
		return "song", songID, nil
	default:
		return "", nil, nil
	}
}
