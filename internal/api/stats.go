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

// registerStats registers the statistics and raw-listen-history endpoints, each scoped by {scope}.
func (s *Server) registerStats(mux authMux) {
	mux.OptionalAuthHandleFunc("GET /stats/{scope}/summary", s.statsSummary)
	mux.OptionalAuthHandleFunc("GET /stats/{scope}/top/{kind}", s.statsTop)
	mux.OptionalAuthHandleFunc("GET /stats/{scope}/activity", s.statsActivity)
	mux.OptionalAuthHandleFunc("GET /stats/{scope}/interest/{type}/{id}", s.statsInterest)
	mux.OptionalAuthHandleFunc("GET /stats/{scope}/clock", s.statsClock)
	mux.OptionalAuthHandleFunc("GET /stats/{scope}/sources", s.statsSources)
	mux.OptionalAuthHandleFunc("GET /stats/{scope}/discovery", s.statsDiscovery)
	mux.OptionalAuthHandleFunc("GET /stats/{scope}/rewind", s.statsRewind)
	mux.OptionalAuthHandleFunc("GET /stats/{scope}/now-playing", s.statsNowPlaying)
	mux.OptionalAuthHandleFunc("GET /stats/{scope}/listens", s.statsListens)
}

// scopeError is a resolveStatsScope failure, ready to write as an HTTP response.
type scopeError struct {
	status int
	detail string
}

// resolveStatsScope parses {scope} into a target user id (nil for "global"), 404ing if viewing it isn't allowed.
func (s *Server) resolveStatsScope(r *http.Request) (userID *int64, serr *scopeError) {
	return s.resolveScope(r, r.PathValue("scope"))
}

// resolveScope parses raw into a target user id (nil for "global"), 404ing if viewing it isn't allowed.
func (s *Server) resolveScope(r *http.Request, raw string) (userID *int64, serr *scopeError) {
	if raw == "global" {
		return nil, nil
	}

	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, &scopeError{http.StatusBadRequest, "scope must be \"global\" or a numeric user id"}
	}
	target, err := s.queries.GetUserByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &scopeError{http.StatusNotFound, "not found"}
	}
	if err != nil {
		return nil, &scopeError{http.StatusInternalServerError, err.Error()}
	}

	caller, authed := auth.UserFromContext(r.Context())
	if target.Public || (authed && (caller.ID == target.ID || caller.IsAdmin)) {
		return &target.ID, nil
	}
	return nil, &scopeError{http.StatusNotFound, "not found"}
}

// statsSummary returns the scoped user's (or every user's, for global scope) overall listening summary.
func (s *Server) statsSummary(w http.ResponseWriter, r *http.Request) {
	userID, serr := s.resolveStatsScope(r)
	if serr != nil {
		fail(w, serr.status, "stats scope", serr.detail)
		return
	}
	tf, err := parseTimeframe(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	data, err := s.stats.Summary(r.Context(), userID, tf)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsTop returns the scoped artist/album/track leaderboard.
func (s *Server) statsTop(w http.ResponseWriter, r *http.Request) {
	userID, serr := s.resolveStatsScope(r)
	if serr != nil {
		fail(w, serr.status, "stats scope", serr.detail)
		return
	}

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
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "listens"
	}
	if sortBy != "listens" && sortBy != "minutes" {
		badRequest(w, "sort must be listens or minutes")
		return
	}
	page, perPage := parsePagination(r)

	data, err := s.stats.Top(r.Context(), userID, kind, tf, artistID, albumID, page, perPage, sortBy)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsActivity returns the scoped zero-filled listen-count time series.
func (s *Server) statsActivity(w http.ResponseWriter, r *http.Request) {
	userID, serr := s.resolveStatsScope(r)
	if serr != nil {
		fail(w, serr.status, "stats scope", serr.detail)
		return
	}
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

	data, err := s.stats.Activity(r.Context(), userID, tf, step, entityType, entityID)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsInterest returns the scoped decay/growth-of-interest graph for one catalog entity.
func (s *Server) statsInterest(w http.ResponseWriter, r *http.Request) {
	userID, serr := s.resolveStatsScope(r)
	if serr != nil {
		fail(w, serr.status, "stats scope", serr.detail)
		return
	}

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
	step := r.URL.Query().Get("step")
	if step == "" {
		step = "day"
	}

	data, err := s.stats.Interest(r.Context(), userID, entityType, entityID, step)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsClock returns the scoped hour x weekday listening-clock heatmap.
func (s *Server) statsClock(w http.ResponseWriter, r *http.Request) {
	userID, serr := s.resolveStatsScope(r)
	if serr != nil {
		fail(w, serr.status, "stats scope", serr.detail)
		return
	}
	tf, err := parseTimeframe(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	data, err := s.stats.Clock(r.Context(), userID, tf)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsSources returns the scoped listen count broken down by source_type.
func (s *Server) statsSources(w http.ResponseWriter, r *http.Request) {
	userID, serr := s.resolveStatsScope(r)
	if serr != nil {
		fail(w, serr.status, "stats scope", serr.detail)
		return
	}
	tf, err := parseTimeframe(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	data, err := s.stats.Sources(r.Context(), userID, tf)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsDiscovery returns the scoped new-vs-repeat listen trend.
func (s *Server) statsDiscovery(w http.ResponseWriter, r *http.Request) {
	userID, serr := s.resolveStatsScope(r)
	if serr != nil {
		fail(w, serr.status, "stats scope", serr.detail)
		return
	}
	tf, err := parseTimeframe(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	step := r.URL.Query().Get("step")
	if step == "" {
		step = "day"
	}
	data, err := s.stats.Discovery(r.Context(), userID, tf, step)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsRewind returns the scoped user's Spotify-Wrapped-style bundle for a period. Not available for global scope.
func (s *Server) statsRewind(w http.ResponseWriter, r *http.Request) {
	userID, serr := s.resolveStatsScope(r)
	if serr != nil {
		fail(w, serr.status, "stats scope", serr.detail)
		return
	}
	if userID == nil {
		badRequest(w, "rewind is not available for global scope")
		return
	}
	tf, err := parseTimeframe(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	data, err := s.stats.Rewind(r.Context(), *userID, tf)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, data)
}

// statsNowPlayingEntry is one now-playing track within a stats scope; User is populated for global scope only.
type statsNowPlayingEntry struct {
	Song       songResponse      `json:"song"`
	StartedAt  time.Time         `json:"started_at"`
	DurationMs *int32            `json:"duration_ms,omitempty"`
	User       *listenerResponse `json:"user,omitempty"`
}

// statsNowPlaying returns what's playing within scope, live and never cached, always as an array.
func (s *Server) statsNowPlaying(w http.ResponseWriter, r *http.Request) {
	userID, serr := s.resolveStatsScope(r)
	if serr != nil {
		fail(w, serr.status, "stats scope", serr.detail)
		return
	}

	if userID == nil {
		rows, err := s.queries.ListNowPlayingAllPublic(r.Context())
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
		songByID := songMapByID(songs)

		out := make([]statsNowPlayingEntry, 0, len(rows))
		for _, row := range rows {
			song, ok := songByID[row.SongID]
			if !ok {
				continue
			}
			out = append(out, statsNowPlayingEntry{
				Song: newSongResponse(song), StartedAt: row.StartedAt.Time, DurationMs: row.DurationMs,
				User: &listenerResponse{ID: &row.UserID, Username: &row.Username, DisplayName: row.DisplayName, ImageURL: imageURL(row.UserImageID)},
			})
		}
		ok(w, out)
		return
	}

	row, err := s.queries.GetNowPlaying(r.Context(), *userID)
	if errors.Is(err, pgx.ErrNoRows) {
		ok(w, []statsNowPlayingEntry{})
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
	ok(w, []statsNowPlayingEntry{{Song: newSongResponse(song), StartedAt: row.StartedAt.Time, DurationMs: row.DurationMs}})
}

// statsListenResponse is one raw listen. User is populated for global scope only.
type statsListenResponse struct {
	ID               int64             `json:"id"`
	Song             songResponse      `json:"song"`
	ListenedAt       time.Time         `json:"listened_at"`
	DurationPlayedMs *int32            `json:"duration_played_ms,omitempty"`
	User             *listenerResponse `json:"user,omitempty"`
}

// listListensResponse is GET /stats/{scope}/listens's paginated payload.
type listListensResponse struct {
	Listens []statsListenResponse `json:"listens"`
	Total   int64                 `json:"total"`
	Page    int                   `json:"page"`
	PerPage int                   `json:"per_page"`
}

// songMapByID indexes songs by id, for attaching full song data onto rows that only carry a song_id.
func songMapByID(songs []db.Song) map[int64]db.Song {
	out := make(map[int64]db.Song, len(songs))
	for _, song := range songs {
		out[song.ID] = song
	}
	return out
}

// statsListens returns the scoped paginated raw listen history, filterable by timeframe/track/album/artist/source.
func (s *Server) statsListens(w http.ResponseWriter, r *http.Request) {
	userID, serr := s.resolveStatsScope(r)
	if serr != nil {
		fail(w, serr.status, "stats scope", serr.detail)
		return
	}
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
	fromTime := pgtype.Timestamptz{Time: from, Valid: true}
	toTime := pgtype.Timestamptz{Time: to, Valid: true}

	if userID == nil {
		rows, err := s.queries.ListListensAllFiltered(r.Context(), db.ListListensAllFilteredParams{
			FromTime: fromTime, ToTime: toTime, SongID: songID, AlbumID: albumID, ArtistID: artistID, SourceType: sourceType,
			MaxRows: int32(perPage), RowOffset: offset,
		})
		if err != nil {
			internalError(w, err.Error())
			return
		}
		total, err := s.queries.CountListensAllFiltered(r.Context(), db.CountListensAllFilteredParams{
			FromTime: fromTime, ToTime: toTime, SongID: songID, AlbumID: albumID, ArtistID: artistID, SourceType: sourceType,
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
		songByID := songMapByID(songs)
		primaryInfo, err := s.songPrimaryInfoMap(r.Context(), songIDs)
		if err != nil {
			internalError(w, err.Error())
			return
		}

		listens := make([]statsListenResponse, 0, len(rows))
		for _, row := range rows {
			song, ok := songByID[row.SongID]
			if !ok {
				continue
			}
			item := statsListenResponse{
				ID: row.ID, Song: withPrimaryInfo(newSongResponse(song), primaryInfo),
				ListenedAt: row.ListenedAt.Time, DurationPlayedMs: row.DurationPlayedMs,
			}
			if row.Public {
				item.User = &listenerResponse{ID: &row.UserID, Username: &row.Username, DisplayName: row.DisplayName, ImageURL: imageURL(row.UserImageID)}
			}
			listens = append(listens, item)
		}
		ok(w, listListensResponse{Listens: listens, Total: total, Page: page, PerPage: perPage})
		return
	}

	params := db.ListListensForUserFilteredParams{
		UserID: *userID, FromTime: fromTime, ToTime: toTime,
		SongID: songID, AlbumID: albumID, ArtistID: artistID, SourceType: sourceType, MaxRows: int32(perPage), RowOffset: offset,
	}
	rows, err := s.queries.ListListensForUserFiltered(r.Context(), params)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	total, err := s.queries.CountListensForUserFiltered(r.Context(), db.CountListensForUserFilteredParams{
		UserID: *userID, FromTime: fromTime, ToTime: toTime,
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
	songByID := songMapByID(songs)
	primaryInfo, err := s.songPrimaryInfoMap(r.Context(), songIDs)
	if err != nil {
		internalError(w, err.Error())
		return
	}

	listens := make([]statsListenResponse, 0, len(rows))
	for _, row := range rows {
		song, ok := songByID[row.SongID]
		if !ok {
			continue
		}
		listens = append(listens, statsListenResponse{
			ID: row.ID, Song: withPrimaryInfo(newSongResponse(song), primaryInfo),
			ListenedAt: row.ListenedAt.Time, DurationPlayedMs: row.DurationPlayedMs,
		})
	}
	ok(w, listListensResponse{Listens: listens, Total: total, Page: page, PerPage: perPage})
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
