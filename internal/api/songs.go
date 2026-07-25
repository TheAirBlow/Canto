package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"Canto/internal/auth"
	"Canto/internal/correlate"
	"Canto/internal/correlate/romanize"
	"Canto/internal/db"
	"Canto/internal/images"
	"Canto/internal/search"
)

// registerSongs registers the song read and curation endpoints.
func (s *Server) registerSongs(mux authMux) {
	mux.HandleFunc("GET /songs/{id}", s.getSong)
	mux.AdminAuthHandleFunc("GET /songs", s.listAllSongs)
	mux.AdminAuthHandleFunc("PUT /songs/{id}", s.updateSong)
	mux.AdminAuthHandleFunc("POST /songs/{id}/merge", s.mergeSong)
	mux.AdminAuthHandleFunc("DELETE /songs/{id}", s.deleteSong)
	mux.AdminAuthHandleFunc("PUT /songs/{id}/image", s.uploadSongImage)
	mux.AdminAuthHandleFunc("PUT /songs/{id}/pin", s.pinSong)
	mux.AdminAuthHandleFunc("DELETE /songs/{id}/pin", s.unpinSong)
	mux.AdminAuthHandleFunc("PUT /songs/{id}/artists", s.setSongArtists)
	mux.HandleFunc("GET /songs/{id}/aliases", s.listSongAliases)
	mux.AdminAuthHandleFunc("POST /songs/{id}/aliases", s.createSongAlias)
	mux.AdminAuthHandleFunc("DELETE /songs/{id}/aliases/{alias_id}", s.deleteSongAlias)
	mux.OptionalAuthHandleFunc("GET /songs/{id}/stats", s.getSongStats)
	mux.OptionalAuthHandleFunc("GET /songs/{id}/listens", s.listSongListens)
	mux.OptionalAuthHandleFunc("GET /songs/{id}/now-playing", s.listSongNowPlaying)
}

// songResponse is the public-facing song shape.
type songResponse struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	DurationMs *int32  `json:"duration_ms,omitempty"`
	ImageURL   *string `json:"image_url,omitempty"`
	Pinned     bool    `json:"pinned"`
	ArtistID   *int64  `json:"artist_id,omitempty"`
	ArtistName *string `json:"artist_name,omitempty"`
	AlbumID    *int64  `json:"album_id,omitempty"`
	AlbumName  *string `json:"album_name,omitempty"`
}

// newSongResponse builds a songResponse from a db.Song.
func newSongResponse(s db.Song) songResponse {
	return songResponse{ID: s.ID, Name: s.Name, DurationMs: s.DurationMs, ImageURL: imageURL(s.ImageID), Pinned: s.Pinned}
}

// songPrimaryInfo is a song's first-billed artist and primary album; IDs are 0 when the song has none.
type songPrimaryInfo struct {
	ArtistID   int64
	ArtistName string
	AlbumID    int64
	AlbumName  string
}

// songPrimaryInfoMap batch-fetches primary artist/album info for songIDs, keyed by song id.
func (s *Server) songPrimaryInfoMap(ctx context.Context, songIDs []int64) (map[int64]songPrimaryInfo, error) {
	rows, err := s.queries.GetSongsPrimaryArtistAlbum(ctx, songIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]songPrimaryInfo, len(rows))
	for _, r := range rows {
		out[r.SongID] = songPrimaryInfo{ArtistID: r.ArtistID, ArtistName: r.ArtistName, AlbumID: r.AlbumID, AlbumName: r.AlbumName}
	}
	return out, nil
}

// withPrimaryInfo fills resp's artist/album fields from info, leaving them unset if info has nothing for resp.ID.
func withPrimaryInfo(resp songResponse, info map[int64]songPrimaryInfo) songResponse {
	pi, ok := info[resp.ID]
	if !ok {
		return resp
	}
	if pi.ArtistID != 0 {
		resp.ArtistID = &pi.ArtistID
		resp.ArtistName = &pi.ArtistName
	}
	if pi.AlbumID != 0 {
		resp.AlbumID = &pi.AlbumID
		resp.AlbumName = &pi.AlbumName
	}
	return resp
}

// songDetailResponse is a song plus its artists and album, returned by GET /songs/{id}.
type songDetailResponse struct {
	songResponse
	Artists     []artistResponse `json:"artists"`
	Album       *albumResponse   `json:"album,omitempty"`
	TrackNumber *int32           `json:"track_number,omitempty"`
}

// getSong returns a single song by id, with its artists and album.
func (s *Server) getSong(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	song, err := s.queries.GetSongByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	artists, err := s.queries.ListArtistsForSong(r.Context(), id)
	if err != nil {
		internalError(w, err.Error())
		return
	}

	resp := songDetailResponse{songResponse: newSongResponse(song), Artists: make([]artistResponse, len(artists))}
	for i, a := range artists {
		resp.Artists[i] = newArtistResponse(a)
	}

	if album, err := s.queries.GetAlbumForSong(r.Context(), id); err == nil {
		ar := newAlbumResponse(db.Album{
			ID: album.ID, Name: album.Name, NameNormalized: album.NameNormalized,
			ReleaseDate: album.ReleaseDate, Description: album.Description, ImageID: album.ImageID, Pinned: album.Pinned,
		})
		resp.Album = &ar
		resp.TrackNumber = album.TrackNumber
	}

	ok(w, resp)
}

// getSongStats returns this song's listening stats, globally or scoped to one user via ?scope=.
func (s *Server) getSongStats(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	userID, serr := s.resolveScope(r, scopeOrGlobal(r))
	if serr != nil {
		fail(w, serr.status, "stats scope", serr.detail)
		return
	}
	stats, err := s.stats.EntitySummary(r.Context(), userID, "song", id)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, stats)
}

// listAllSongs returns a cursor-paginated page of the full song catalog, ordered by id, for admin browsing.
func (s *Server) listAllSongs(w http.ResponseWriter, r *http.Request) {
	after, limit, err := parseCursorPage(r, catalogPageDefault, catalogPageMax)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	rows, err := s.queries.ListSongs(r.Context(), db.ListSongsParams{After: after, MaxRows: int32(limit)})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	resp := make([]songResponse, len(rows))
	for i, sg := range rows {
		resp[i] = newSongResponse(sg)
	}
	ok(w, resp)
}

// listSongListens returns this song's listens, globally or scoped to one user via ?scope=, anonymizing private users.
func (s *Server) listSongListens(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	userID, serr := s.resolveScope(r, scopeOrGlobal(r))
	if serr != nil {
		fail(w, serr.status, "stats scope", serr.detail)
		return
	}
	page, perPage := parsePagination(r)
	offset := int32((page - 1) * perPage)

	rows, err := s.queries.ListListensForSong(r.Context(), db.ListListensForSongParams{SongID: id, UserID: userID, MaxRows: int32(perPage), RowOffset: offset})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	total, err := s.queries.CountListensForSong(r.Context(), db.CountListensForSongParams{SongID: id, UserID: userID})
	if err != nil {
		internalError(w, err.Error())
		return
	}

	listens := make([]listenResponse, len(rows))
	for i, row := range rows {
		listens[i] = listenResponse{ListenedAt: row.ListenedAt.Time}
		if row.Public || userID != nil {
			listens[i].User = &listenerResponse{ID: &row.UserID, Username: &row.Username, DisplayName: row.DisplayName, ImageURL: imageURL(row.UserImageID)}
		}
	}
	ok(w, listensPage{Listens: listens, Total: total, Page: page, PerPage: perPage})
}

// listSongNowPlaying returns who's currently listening to this song, globally or scoped to one user via ?scope=.
func (s *Server) listSongNowPlaying(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	userID, serr := s.resolveScope(r, scopeOrGlobal(r))
	if serr != nil {
		fail(w, serr.status, "stats scope", serr.detail)
		return
	}
	rows, err := s.queries.ListNowPlayingForSong(r.Context(), db.ListNowPlayingForSongParams{SongID: id, UserID: userID})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	out := make([]listeningNowResponse, len(rows))
	for i, row := range rows {
		out[i] = listeningNowResponse{
			User:      listenerResponse{ID: &row.UserID, Username: &row.Username, DisplayName: row.DisplayName, ImageURL: imageURL(row.UserImageID)},
			StartedAt: row.StartedAt.Time,
		}
	}
	ok(w, out)
}

// updateSong edits an existing song's name.
func (s *Server) updateSong(w http.ResponseWriter, r *http.Request) {
	admin, _ := auth.UserFromContext(r.Context())

	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	var req entityEditRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(w, "name is required")
		return
	}

	song, err := s.queries.UpdateSong(r.Context(), db.UpdateSongParams{
		ID: id, Name: req.Name, NameNormalized: correlate.NormalizeName(req.Name), NameRomanized: romanize.Romanize(req.Name),
	})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ctx := r.Context()
	currentArtists, err := s.queries.ListArtistsForSong(ctx, id)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	artistIDs := make([]int64, len(currentArtists))
	artistNames := make([]string, len(currentArtists))
	for i, a := range currentArtists {
		artistIDs[i], artistNames[i] = a.ID, a.Name
	}
	var albumID *int64
	var albumName string
	if album, err := s.queries.GetAlbumForSong(ctx, id); err == nil {
		albumID, albumName = &album.ID, album.Name
	}
	s.search.Upsert(ctx, "songs", search.Document{
		ID: song.ID, EntityType: "song", Name: song.Name, NameNormalized: song.NameNormalized, NameRomanized: song.NameRomanized,
		ArtistIDs: artistIDs, ArtistNames: artistNames, AlbumID: albumID, AlbumName: albumName,
	})
	slog.Info("admin: song updated", "admin", admin.Username, "id", song.ID, "name", song.Name)
	ok(w, newSongResponse(song))
}

// mergeSong merges the path song into req.Into, repointing every reference and deleting the loser.
func (s *Server) mergeSong(w http.ResponseWriter, r *http.Request) {
	admin, _ := auth.UserFromContext(r.Context())

	oldID, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	var req mergeRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	if req.Into == oldID {
		badRequest(w, "into must differ from the merged song")
		return
	}

	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)

	if err := correlate.MergeEntity(ctx, q, db.EntityTypeSong, oldID, req.Into); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internalError(w, err.Error())
		return
	}

	s.search.Delete(ctx, "songs", oldID)
	slog.Info("admin: song merged", "admin", admin.Username, "from", oldID, "into", req.Into)
	w.WriteHeader(http.StatusNoContent)
}

// deleteSong permanently removes a song and every row keyed to it.
func (s *Server) deleteSong(w http.ResponseWriter, r *http.Request) {
	admin, _ := auth.UserFromContext(r.Context())

	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}

	ctx := r.Context()
	song, err := s.queries.GetSongByID(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)

	if err := deletePolymorphicStats(ctx, q, db.EntityTypeSong, id); err != nil {
		internalError(w, err.Error())
		return
	}
	if _, err := q.DeleteSong(ctx, id); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internalError(w, err.Error())
		return
	}

	images.DeleteIfSet(song.ImageID)
	s.search.Delete(ctx, "songs", id)
	slog.Info("admin: song deleted", "admin", admin.Username, "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// uploadSongImage replaces a song's cover image from a multipart upload, pinning it against auto-refresh.
func (s *Server) uploadSongImage(w http.ResponseWriter, r *http.Request) {
	admin, _ := auth.UserFromContext(r.Context())

	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	song, err := s.queries.GetSongByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	imageID, err := storeUploadedImage(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	if _, err := s.queries.SetSongImage(r.Context(), db.SetSongImageParams{ID: id, ImageID: uuidParam(imageID)}); err != nil {
		internalError(w, err.Error())
		return
	}
	images.DeleteIfSet(song.ImageID)
	slog.Info("admin: song image replaced", "admin", admin.Username, "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// pinSong marks a song as manually curated, so refresh/enrichment never auto-overwrites its metadata/image.
func (s *Server) pinSong(w http.ResponseWriter, r *http.Request) {
	s.setSongPinned(w, r, true)
}

// unpinSong releases a song back to normal auto-refresh/enrichment.
func (s *Server) unpinSong(w http.ResponseWriter, r *http.Request) {
	s.setSongPinned(w, r, false)
}

// setSongPinned sets a song's pinned flag and returns its updated state.
func (s *Server) setSongPinned(w http.ResponseWriter, r *http.Request, pinned bool) {
	admin, _ := auth.UserFromContext(r.Context())

	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	song, err := s.queries.SetSongPinned(r.Context(), db.SetSongPinnedParams{ID: id, Pinned: pinned})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	slog.Info("admin: song pinned state changed", "admin", admin.Username, "id", id, "pinned", pinned)
	ok(w, newSongResponse(song))
}

// setSongArtists replaces a song's full artist linkage set, in order; the first id is primary.
func (s *Server) setSongArtists(w http.ResponseWriter, r *http.Request) {
	admin, _ := auth.UserFromContext(r.Context())

	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	var req setArtistsRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	if len(req.ArtistIDs) == 0 {
		badRequest(w, "artist_ids must not be empty")
		return
	}

	ctx := r.Context()
	if err := s.queries.DeleteSongArtists(ctx, id); err != nil {
		internalError(w, err.Error())
		return
	}
	for i, artistID := range req.ArtistIDs {
		if err := s.queries.LinkSongArtist(ctx, db.LinkSongArtistParams{SongID: id, ArtistID: artistID, Position: int16(i)}); err != nil {
			internalError(w, err.Error())
			return
		}
	}
	artists, err := s.queries.GetArtistsByIDs(ctx, req.ArtistIDs)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	artistNames := make([]string, len(artists))
	for i, a := range artists {
		artistNames[i] = a.Name
	}
	s.search.Upsert(ctx, "songs", search.Document{ID: id, EntityType: "song", ArtistIDs: req.ArtistIDs, ArtistNames: artistNames})
	slog.Info("admin: song artists reassigned", "admin", admin.Username, "id", id, "artist_ids", req.ArtistIDs)
	w.WriteHeader(http.StatusNoContent)
}

// listSongAliases lists a song's known aliases.
func (s *Server) listSongAliases(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	aliases, err := s.queries.ListAliasesForEntity(r.Context(), db.ListAliasesForEntityParams{EntityType: db.EntityTypeSong, EntityID: id})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	resp := make([]aliasResponse, len(aliases))
	for i, a := range aliases {
		resp[i] = aliasResponse{ID: a.ID, Alias: a.Alias}
	}
	ok(w, resp)
}

// createSongAlias adds a searchable alternate name for a song.
func (s *Server) createSongAlias(w http.ResponseWriter, r *http.Request) {
	admin, _ := auth.UserFromContext(r.Context())

	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	var req createAliasRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	req.Alias = strings.TrimSpace(req.Alias)
	if req.Alias == "" {
		badRequest(w, "alias is required")
		return
	}

	alias, err := s.queries.CreateAlias(r.Context(), db.CreateAliasParams{EntityType: db.EntityTypeSong, EntityID: id, Alias: req.Alias})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	slog.Info("admin: song alias added", "admin", admin.Username, "id", id, "alias", req.Alias)
	created(w, aliasResponse{ID: alias.ID, Alias: alias.Alias})
}

// deleteSongAlias removes one of a song's aliases.
func (s *Server) deleteSongAlias(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	aliasID, err := strconv.ParseInt(r.PathValue("alias_id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid alias_id in path")
		return
	}

	rows, err := s.queries.DeleteAlias(r.Context(), db.DeleteAliasParams{ID: aliasID, EntityType: db.EntityTypeSong, EntityID: id})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	if rows == 0 {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
