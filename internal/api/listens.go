package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"Canto/internal/auth"
	"Canto/internal/db"
	"Canto/internal/rollup"
)

// registerListens registers the self-service listen-deletion endpoint.
func (s *Server) registerListens(mux authMux) {
	mux.CookieAuthHandleFunc("DELETE /listens/{id}", s.deleteListen)
}

// deleteListen removes one of the caller's own listens immediately and queues its rollup reversal for the background Unrecorder.
func (s *Server) deleteListen(w http.ResponseWriter, r *http.Request) {
	caller, _ := auth.UserFromContext(r.Context())

	id, err := pathID(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}

	ctx := r.Context()
	deleted, err := s.queries.DeleteListenForUser(ctx, db.DeleteListenForUserParams{ID: id, UserID: caller.ID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		internalError(w, err.Error())
		return
	}

	song, err := s.queries.GetSongByID(ctx, deleted.SongID)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	var songDurationMs, playedMs int32
	if song.DurationMs != nil {
		songDurationMs = *song.DurationMs
	}
	if deleted.DurationPlayedMs != nil {
		playedMs = *deleted.DurationPlayedMs
	} else {
		playedMs = songDurationMs
	}

	var albumID *int64
	if album, err := s.queries.GetAlbumForSong(ctx, deleted.SongID); err == nil {
		albumID = &album.ID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		internalError(w, err.Error())
		return
	}
	artists, err := s.queries.ListArtistsForSong(ctx, deleted.SongID)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	artistIDs := make([]int64, len(artists))
	for i, a := range artists {
		artistIDs[i] = a.ID
	}

	s.unrecorder.Enqueue(rollup.UnrecordedListen{
		UserID: caller.ID, SongID: deleted.SongID, AlbumID: albumID, ArtistIDs: artistIDs,
		ListenedAt: deleted.ListenedAt.Time, PlayedMs: playedMs, SongDurationMs: songDurationMs,
	})
	w.WriteHeader(http.StatusNoContent)
}

// listenerResponse is a listen's or now-playing entry's attributed user, omitted entirely when private.
type listenerResponse struct {
	ID          *int64  `json:"id,omitempty"`
	Username    *string `json:"username,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
	ImageURL    *string `json:"image_url,omitempty"`
}

// listenResponse is one listen of a catalog entity by any user, anonymized if that user is private.
type listenResponse struct {
	ListenedAt time.Time         `json:"listened_at"`
	User       *listenerResponse `json:"user,omitempty"`
}

// listeningNowResponse is one user currently listening to a catalog entity; private users never appear here.
type listeningNowResponse struct {
	User      listenerResponse `json:"user"`
	StartedAt time.Time        `json:"started_at"`
}

// listensPage is a paginated listenResponse list.
type listensPage struct {
	Listens []listenResponse `json:"listens"`
	Total   int64            `json:"total"`
	Page    int              `json:"page"`
	PerPage int              `json:"per_page"`
}
