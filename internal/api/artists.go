package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/auth"
	"Canto/internal/correlate"
	"Canto/internal/db"
	"Canto/internal/images"
	"Canto/internal/search"
)

// registerArtists registers the artist blacklist and curation endpoints.
func (s *Server) registerArtists(mux authMux) {
	mux.CookieAuthHandleFunc("GET /artists/blacklist", s.listBlacklistedArtists)
	mux.CookieAuthHandleFunc("PUT /artists/{id}/blacklist", s.blacklistArtist)
	mux.CookieAuthHandleFunc("DELETE /artists/{id}/blacklist", s.unblacklistArtist)
	mux.HandleFunc("GET /artists/{id}", s.getArtist)
	mux.AdminAuthHandleFunc("POST /artists", s.createArtist)
	mux.AdminAuthHandleFunc("PUT /artists/{id}", s.updateArtist)
	mux.AdminAuthHandleFunc("POST /artists/{id}/merge", s.mergeArtist)
	mux.AdminAuthHandleFunc("PUT /artists/{id}/image", s.uploadArtistImage)
	mux.AdminAuthHandleFunc("PUT /artists/{id}/pin", s.pinArtist)
	mux.AdminAuthHandleFunc("DELETE /artists/{id}/pin", s.unpinArtist)
	mux.HandleFunc("GET /artists/{id}/aliases", s.listArtistAliases)
	mux.AdminAuthHandleFunc("POST /artists/{id}/aliases", s.createArtistAlias)
	mux.AdminAuthHandleFunc("DELETE /artists/{id}/aliases/{alias_id}", s.deleteArtistAlias)
}

// artistResponse is the public-facing artist shape.
type artistResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	ImageURL    *string `json:"image_url,omitempty"`
	Pinned      bool    `json:"pinned"`
}

// newArtistResponse builds an artistResponse from a db.Artist.
func newArtistResponse(a db.Artist) artistResponse {
	return artistResponse{ID: a.ID, Name: a.Name, Description: a.Description, ImageURL: imageURL(a.ImageID), Pinned: a.Pinned}
}

// artistDetailResponse is an artist plus its full discography, returned by GET /artists/{id}.
type artistDetailResponse struct {
	artistResponse
	Albums []albumResponse `json:"albums"`
	Songs  []songResponse  `json:"songs"`
}

// getArtist returns a single artist by id, with its full discography.
func (s *Server) getArtist(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	artist, err := s.queries.GetArtistByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	albums, err := s.queries.ListAlbumsForArtist(r.Context(), id)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	songs, err := s.queries.ListSongsForArtist(r.Context(), id)
	if err != nil {
		internalError(w, err.Error())
		return
	}

	resp := artistDetailResponse{artistResponse: newArtistResponse(artist), Albums: make([]albumResponse, len(albums)), Songs: make([]songResponse, len(songs))}
	for i, a := range albums {
		resp.Albums[i] = newAlbumResponse(a)
	}
	for i, sg := range songs {
		resp.Songs[i] = newSongResponse(sg)
	}
	ok(w, resp)
}

// entityEditRequest is the JSON body for creating or editing an artist/album/song.
type entityEditRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// createArtist manually creates a new artist.
func (s *Server) createArtist(w http.ResponseWriter, r *http.Request) {
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

	artist, err := s.queries.CreateArtist(r.Context(), db.CreateArtistParams{
		Name: req.Name, NameNormalized: correlate.NormalizeName(req.Name),
	})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	s.search.Upsert(r.Context(), "artists", search.Document{ID: artist.ID, EntityType: "artist", Name: artist.Name, NameNormalized: artist.NameNormalized})
	slog.Info("admin: artist created", "admin", admin.Username, "id", artist.ID, "name", artist.Name)
	created(w, newArtistResponse(artist))
}

// updateArtist edits an existing artist's name/description.
func (s *Server) updateArtist(w http.ResponseWriter, r *http.Request) {
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

	before, err := s.queries.GetArtistByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	artist, err := s.queries.UpdateArtist(r.Context(), db.UpdateArtistParams{
		ID: id, Name: req.Name, NameNormalized: correlate.NormalizeName(req.Name), Description: req.Description,
	})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	s.search.Upsert(r.Context(), "artists", search.Document{ID: artist.ID, EntityType: "artist", Name: artist.Name, NameNormalized: artist.NameNormalized})
	if artist.Name != before.Name {
		s.search.CascadeArtistRename(artist.ID)
	}
	slog.Info("admin: artist updated", "admin", admin.Username, "id", artist.ID, "name", artist.Name)
	ok(w, newArtistResponse(artist))
}

// mergeRequest is the JSON body for merging one entity into another.
type mergeRequest struct {
	Into int64 `json:"into"`
}

// mergeArtist merges the path artist into req.Into, repointing every reference and deleting the loser.
func (s *Server) mergeArtist(w http.ResponseWriter, r *http.Request) {
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
		badRequest(w, "into must differ from the merged artist")
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

	if err := q.RepointSourcesForMerge(ctx, db.RepointSourcesForMergeParams{NewEntityID: req.Into, EntityType: db.EntityTypeArtist, OldEntityID: oldID}); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := q.RepointAlbumArtistsForArtistMerge(ctx, db.RepointAlbumArtistsForArtistMergeParams{NewArtistID: req.Into, OldArtistID: oldID}); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := q.DeleteRemainingAlbumArtistsForArtist(ctx, oldID); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := q.RepointSongArtistsForArtistMerge(ctx, db.RepointSongArtistsForArtistMergeParams{NewArtistID: req.Into, OldArtistID: oldID}); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := q.DeleteRemainingSongArtistsForArtist(ctx, oldID); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := q.RepointBlacklistForArtistMerge(ctx, db.RepointBlacklistForArtistMergeParams{NewArtistID: req.Into, OldArtistID: oldID}); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := q.DeleteRemainingBlacklistForArtist(ctx, oldID); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := q.RepointAliases(ctx, db.RepointAliasesParams{EntityType: db.EntityTypeArtist, OldEntityID: oldID, NewEntityID: req.Into}); err != nil {
		internalError(w, err.Error())
		return
	}
	if _, err := q.DeleteArtist(ctx, oldID); err != nil {
		internalError(w, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internalError(w, err.Error())
		return
	}

	s.search.Delete(ctx, "artists", oldID)
	slog.Info("admin: artist merged", "admin", admin.Username, "from", oldID, "into", req.Into)
	w.WriteHeader(http.StatusNoContent)
}

// uploadArtistImage replaces an artist's cover image from a multipart upload, pinning it against auto-refresh.
func (s *Server) uploadArtistImage(w http.ResponseWriter, r *http.Request) {
	admin, _ := auth.UserFromContext(r.Context())

	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	artist, err := s.queries.GetArtistByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	imageID, err := storeUploadedImage(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	if _, err := s.queries.SetArtistImage(r.Context(), db.SetArtistImageParams{ID: id, ImageID: uuidParam(imageID)}); err != nil {
		internalError(w, err.Error())
		return
	}
	images.DeleteIfSet(artist.ImageID)
	slog.Info("admin: artist image replaced", "admin", admin.Username, "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// pinArtist marks an artist as manually curated, so refresh/enrichment never auto-overwrites its metadata/image.
func (s *Server) pinArtist(w http.ResponseWriter, r *http.Request) {
	s.setArtistPinned(w, r, true)
}

// unpinArtist releases an artist back to normal auto-refresh/enrichment.
func (s *Server) unpinArtist(w http.ResponseWriter, r *http.Request) {
	s.setArtistPinned(w, r, false)
}

// setArtistPinned sets an artist's pinned flag and returns its updated state.
func (s *Server) setArtistPinned(w http.ResponseWriter, r *http.Request, pinned bool) {
	admin, _ := auth.UserFromContext(r.Context())

	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	artist, err := s.queries.SetArtistPinned(r.Context(), db.SetArtistPinnedParams{ID: id, Pinned: pinned})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	slog.Info("admin: artist pinned state changed", "admin", admin.Username, "id", id, "pinned", pinned)
	ok(w, newArtistResponse(artist))
}

// aliasResponse is the public-facing alias shape.
type aliasResponse struct {
	ID    int64  `json:"id"`
	Alias string `json:"alias"`
}

// listArtistAliases lists an artist's known aliases.
func (s *Server) listArtistAliases(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	aliases, err := s.queries.ListAliasesForEntity(r.Context(), db.ListAliasesForEntityParams{EntityType: db.EntityTypeArtist, EntityID: id})
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

// createAliasRequest is the JSON body for adding an alias.
type createAliasRequest struct {
	Alias string `json:"alias"`
}

// createArtistAlias adds a searchable alternate name for an artist.
func (s *Server) createArtistAlias(w http.ResponseWriter, r *http.Request) {
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

	alias, err := s.queries.CreateAlias(r.Context(), db.CreateAliasParams{EntityType: db.EntityTypeArtist, EntityID: id, Alias: req.Alias})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	slog.Info("admin: artist alias added", "admin", admin.Username, "id", id, "alias", req.Alias)
	created(w, aliasResponse{ID: alias.ID, Alias: alias.Alias})
}

// deleteArtistAlias removes one of an artist's aliases.
func (s *Server) deleteArtistAlias(w http.ResponseWriter, r *http.Request) {
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

	rows, err := s.queries.DeleteAlias(r.Context(), db.DeleteAliasParams{ID: aliasID, EntityType: db.EntityTypeArtist, EntityID: id})
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

// listBlacklistedArtists lists the caller's blacklisted artists, excluded from their artist-related statistics.
func (s *Server) listBlacklistedArtists(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	artists, err := s.queries.ListBlacklistedArtists(r.Context(), user.ID)
	if err != nil {
		internalError(w, err.Error())
		return
	}

	resp := make([]artistResponse, len(artists))
	for i, a := range artists {
		resp[i] = newArtistResponse(a)
	}
	ok(w, resp)
}

// blacklistArtist excludes an artist from the caller's artist-related statistics.
func (s *Server) blacklistArtist(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	artistID, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}

	if _, err := s.queries.GetArtistByID(r.Context(), artistID); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := s.queries.BlacklistArtist(r.Context(), db.BlacklistArtistParams{UserID: user.ID, ArtistID: artistID}); err != nil {
		internalError(w, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// unblacklistArtist removes an artist from the caller's blacklist.
func (s *Server) unblacklistArtist(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	artistID, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}

	if err := s.queries.UnblacklistArtist(r.Context(), db.UnblacklistArtistParams{UserID: user.ID, ArtistID: artistID}); err != nil {
		internalError(w, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// pathID parses the "id" path value as an int64.
func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// storeUploadedImage reads the "image" multipart field off r, caches it, and returns its new id.
func storeUploadedImage(r *http.Request) (*uuid.UUID, error) {
	file, _, err := r.FormFile("image")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	id := uuid.New()
	if err := images.Store(id, file); err != nil {
		return nil, err
	}
	return &id, nil
}

// uuidParam converts an optional uuid.UUID into a nullable pgtype.UUID.
func uuidParam(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}
