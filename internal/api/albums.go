package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"Canto/internal/auth"
	"Canto/internal/correlate"
	"Canto/internal/db"
	"Canto/internal/images"
	"Canto/internal/search"
)

// registerAlbums registers the album read and curation endpoints.
func (s *Server) registerAlbums(mux authMux) {
	mux.HandleFunc("GET /albums/{id}", s.getAlbum)
	mux.AdminAuthHandleFunc("POST /albums", s.createAlbum)
	mux.AdminAuthHandleFunc("PUT /albums/{id}", s.updateAlbum)
	mux.AdminAuthHandleFunc("POST /albums/{id}/merge", s.mergeAlbum)
	mux.AdminAuthHandleFunc("PUT /albums/{id}/image", s.uploadAlbumImage)
	mux.AdminAuthHandleFunc("PUT /albums/{id}/pin", s.pinAlbum)
	mux.AdminAuthHandleFunc("DELETE /albums/{id}/pin", s.unpinAlbum)
	mux.AdminAuthHandleFunc("PUT /albums/{id}/artists", s.setAlbumArtists)
	mux.HandleFunc("GET /albums/{id}/aliases", s.listAlbumAliases)
	mux.AdminAuthHandleFunc("POST /albums/{id}/aliases", s.createAlbumAlias)
	mux.AdminAuthHandleFunc("DELETE /albums/{id}/aliases/{alias_id}", s.deleteAlbumAlias)
	mux.HandleFunc("GET /albums/{id}/listens", s.listAlbumListens)
	mux.HandleFunc("GET /albums/{id}/now-playing", s.listAlbumNowPlaying)
}

// albumResponse is the public-facing album shape.
type albumResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	ImageURL    *string `json:"image_url,omitempty"`
	Pinned      bool    `json:"pinned"`
}

// newAlbumResponse builds an albumResponse from a db.Album.
func newAlbumResponse(a db.Album) albumResponse {
	return albumResponse{ID: a.ID, Name: a.Name, Description: a.Description, ImageURL: imageURL(a.ImageID), Pinned: a.Pinned}
}

// trackResponse is one track in an album's listing.
type trackResponse struct {
	songResponse
	TrackNumber *int32 `json:"track_number,omitempty"`
}

// albumDetailResponse is an album plus its artists and tracklist, returned by GET /albums/{id}.
type albumDetailResponse struct {
	albumResponse
	Artists []artistResponse `json:"artists"`
	Tracks  []trackResponse  `json:"tracks"`
	Stats   json.RawMessage  `json:"stats,omitempty"`
}

// getAlbum returns a single album by id, with its artists, tracklist, and global listening stats.
func (s *Server) getAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	album, err := s.queries.GetAlbumByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	artists, err := s.queries.ListArtistsForAlbum(r.Context(), id)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	tracks, err := s.queries.ListSongsForAlbum(r.Context(), id)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	stats, err := s.stats.EntitySummary(r.Context(), "album", id)
	if err != nil {
		slog.Warn("album stats fetch failed", "id", id, "err", err)
	}

	resp := albumDetailResponse{
		albumResponse: newAlbumResponse(album), Artists: make([]artistResponse, len(artists)),
		Tracks: make([]trackResponse, len(tracks)), Stats: stats,
	}
	for i, a := range artists {
		resp.Artists[i] = newArtistResponse(a)
	}
	for i, t := range tracks {
		resp.Tracks[i] = trackResponse{
			songResponse: songResponse{ID: t.ID, Name: t.Name, DurationMs: t.DurationMs, ImageURL: imageURL(t.ImageID)},
			TrackNumber:  t.TrackNumber,
		}
	}
	ok(w, resp)
}

// listAlbumListens returns every listen of this album by any user, newest first, anonymizing private users.
func (s *Server) listAlbumListens(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	page, perPage := parsePagination(r)
	offset := int32((page - 1) * perPage)

	rows, err := s.queries.ListListensForAlbum(r.Context(), db.ListListensForAlbumParams{AlbumID: id, MaxRows: int32(perPage), RowOffset: offset})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	total, err := s.queries.CountListensForAlbum(r.Context(), id)
	if err != nil {
		internalError(w, err.Error())
		return
	}

	listens := make([]listenResponse, len(rows))
	for i, row := range rows {
		listens[i] = listenResponse{ListenedAt: row.ListenedAt.Time}
		if row.Public {
			listens[i].User = &listenerResponse{ID: &row.UserID, Username: &row.Username, DisplayName: row.DisplayName, ImageURL: imageURL(row.UserImageID)}
		}
	}
	ok(w, listensPage{Listens: listens, Total: total, Page: page, PerPage: perPage})
}

// listAlbumNowPlaying returns every public user currently listening to this album.
func (s *Server) listAlbumNowPlaying(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	rows, err := s.queries.ListNowPlayingForAlbum(r.Context(), id)
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

// createAlbum manually creates a new album.
func (s *Server) createAlbum(w http.ResponseWriter, r *http.Request) {
	admin, _ := auth.UserFromContext(r.Context())

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

	album, err := s.queries.CreateAlbum(r.Context(), db.CreateAlbumParams{
		Name: req.Name, NameNormalized: correlate.NormalizeName(req.Name),
	})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	s.search.Upsert(r.Context(), "albums", search.Document{ID: album.ID, EntityType: "album", Name: album.Name, NameNormalized: album.NameNormalized})
	slog.Info("admin: album created", "admin", admin.Username, "id", album.ID, "name", album.Name)
	created(w, newAlbumResponse(album))
}

// updateAlbum edits an existing album's name/description.
func (s *Server) updateAlbum(w http.ResponseWriter, r *http.Request) {
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

	before, err := s.queries.GetAlbumByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	album, err := s.queries.UpdateAlbum(r.Context(), db.UpdateAlbumParams{
		ID: id, Name: req.Name, NameNormalized: correlate.NormalizeName(req.Name), Description: req.Description,
	})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	currentArtists, err := s.queries.ListArtistsForAlbum(r.Context(), id)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	artistIDs := make([]int64, len(currentArtists))
	artistNames := make([]string, len(currentArtists))
	for i, a := range currentArtists {
		artistIDs[i], artistNames[i] = a.ID, a.Name
	}
	s.search.Upsert(r.Context(), "albums", search.Document{
		ID: album.ID, EntityType: "album", Name: album.Name, NameNormalized: album.NameNormalized,
		ArtistIDs: artistIDs, ArtistNames: artistNames,
	})
	if album.Name != before.Name {
		s.search.CascadeAlbumRename(album.ID)
	}
	slog.Info("admin: album updated", "admin", admin.Username, "id", album.ID, "name", album.Name)
	ok(w, newAlbumResponse(album))
}

// mergeAlbum merges the path album into req.Into, repointing every reference and deleting the loser.
func (s *Server) mergeAlbum(w http.ResponseWriter, r *http.Request) {
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
		badRequest(w, "into must differ from the merged album")
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

	if err := q.RepointSourcesForMerge(ctx, db.RepointSourcesForMergeParams{NewEntityID: req.Into, EntityType: db.EntityTypeAlbum, OldEntityID: oldID}); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := q.RepointAlbumArtistsForAlbumMerge(ctx, db.RepointAlbumArtistsForAlbumMergeParams{NewAlbumID: req.Into, OldAlbumID: oldID}); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := q.DeleteRemainingAlbumArtistsForAlbum(ctx, oldID); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := q.RepointSongAlbumsForAlbumMerge(ctx, db.RepointSongAlbumsForAlbumMergeParams{NewAlbumID: req.Into, OldAlbumID: oldID}); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := q.DeleteRemainingSongAlbumsForAlbum(ctx, oldID); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := q.RepointAliases(ctx, db.RepointAliasesParams{EntityType: db.EntityTypeAlbum, OldEntityID: oldID, NewEntityID: req.Into}); err != nil {
		internalError(w, err.Error())
		return
	}
	if _, err := q.DeleteAlbum(ctx, oldID); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internalError(w, err.Error())
		return
	}

	s.search.Delete(ctx, "albums", oldID)
	slog.Info("admin: album merged", "admin", admin.Username, "from", oldID, "into", req.Into)
	w.WriteHeader(http.StatusNoContent)
}

// uploadAlbumImage replaces an album's cover image from a multipart upload, pinning it against auto-refresh.
func (s *Server) uploadAlbumImage(w http.ResponseWriter, r *http.Request) {
	admin, _ := auth.UserFromContext(r.Context())

	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	album, err := s.queries.GetAlbumByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	imageID, err := storeUploadedImage(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	if _, err := s.queries.SetAlbumImage(r.Context(), db.SetAlbumImageParams{ID: id, ImageID: uuidParam(imageID)}); err != nil {
		internalError(w, err.Error())
		return
	}
	images.DeleteIfSet(album.ImageID)
	slog.Info("admin: album image replaced", "admin", admin.Username, "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// pinAlbum marks an album as manually curated, so refresh/enrichment never auto-overwrites its metadata/image.
func (s *Server) pinAlbum(w http.ResponseWriter, r *http.Request) {
	s.setAlbumPinned(w, r, true)
}

// unpinAlbum releases an album back to normal auto-refresh/enrichment.
func (s *Server) unpinAlbum(w http.ResponseWriter, r *http.Request) {
	s.setAlbumPinned(w, r, false)
}

// setAlbumPinned sets an album's pinned flag and returns its updated state.
func (s *Server) setAlbumPinned(w http.ResponseWriter, r *http.Request, pinned bool) {
	admin, _ := auth.UserFromContext(r.Context())

	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	album, err := s.queries.SetAlbumPinned(r.Context(), db.SetAlbumPinnedParams{ID: id, Pinned: pinned})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	slog.Info("admin: album pinned state changed", "admin", admin.Username, "id", id, "pinned", pinned)
	ok(w, newAlbumResponse(album))
}

// setArtistsRequest is the JSON body for replacing an album/song's ordered artist list.
type setArtistsRequest struct {
	ArtistIDs []int64 `json:"artist_ids"`
}

// setAlbumArtists replaces an album's full artist linkage set, in order; the first id is primary.
func (s *Server) setAlbumArtists(w http.ResponseWriter, r *http.Request) {
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
	if err := s.queries.DeleteAlbumArtists(ctx, id); err != nil {
		internalError(w, err.Error())
		return
	}
	for i, artistID := range req.ArtistIDs {
		if err := s.queries.LinkAlbumArtist(ctx, db.LinkAlbumArtistParams{AlbumID: id, ArtistID: artistID, Position: int16(i)}); err != nil {
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
	s.search.Upsert(ctx, "albums", search.Document{ID: id, EntityType: "album", ArtistIDs: req.ArtistIDs, ArtistNames: artistNames})
	slog.Info("admin: album artists reassigned", "admin", admin.Username, "id", id, "artist_ids", req.ArtistIDs)
	w.WriteHeader(http.StatusNoContent)
}

// listAlbumAliases lists an album's known aliases.
func (s *Server) listAlbumAliases(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	aliases, err := s.queries.ListAliasesForEntity(r.Context(), db.ListAliasesForEntityParams{EntityType: db.EntityTypeAlbum, EntityID: id})
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

// createAlbumAlias adds a searchable alternate name for an album.
func (s *Server) createAlbumAlias(w http.ResponseWriter, r *http.Request) {
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

	alias, err := s.queries.CreateAlias(r.Context(), db.CreateAliasParams{EntityType: db.EntityTypeAlbum, EntityID: id, Alias: req.Alias})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	slog.Info("admin: album alias added", "admin", admin.Username, "id", id, "alias", req.Alias)
	created(w, aliasResponse{ID: alias.ID, Alias: alias.Alias})
}

// deleteAlbumAlias removes one of an album's aliases.
func (s *Server) deleteAlbumAlias(w http.ResponseWriter, r *http.Request) {
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

	rows, err := s.queries.DeleteAlias(r.Context(), db.DeleteAliasParams{ID: aliasID, EntityType: db.EntityTypeAlbum, EntityID: id})
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
