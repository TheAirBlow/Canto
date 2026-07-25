package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"Canto/internal/auth"
	"Canto/internal/db"
	"Canto/internal/rollup"
)

// registerAdmin registers admin-only endpoints.
func (s *Server) registerAdmin(mux authMux) {
	mux.AdminAuthHandleFunc("POST /admin/invites", s.createInvite)
	mux.AdminAuthHandleFunc("GET /admin/invites", s.listInvites)
	mux.AdminAuthHandleFunc("DELETE /admin/invites/{id}", s.deleteInvite)
	mux.AdminAuthHandleFunc("POST /admin/reindex", s.reindex)
	mux.AdminAuthHandleFunc("POST /admin/stats/recompute", s.recomputeStats)
}

// inviteResponse is the public-facing invite shape.
type inviteResponse struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	MaxUses   *int32    `json:"max_uses"`
	UsesCount int32     `json:"uses_count"`
	CreatedAt time.Time `json:"created_at"`
}

// newInviteResponse builds an inviteResponse from a db.Invite.
func newInviteResponse(i db.Invite) inviteResponse {
	return inviteResponse{ID: i.ID, Code: i.Code, Name: i.Name, MaxUses: i.MaxUses, UsesCount: i.UsesCount, CreatedAt: i.CreatedAt.Time}
}

// createInviteRequest is the JSON body for creating an invite.
type createInviteRequest struct {
	Name    string `json:"name"`
	MaxUses *int32 `json:"max_uses"`
}

// createInvite mints a new invite code.
func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	var req createInviteRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(w, "name is required")
		return
	}
	if req.MaxUses != nil && *req.MaxUses <= 0 {
		badRequest(w, "max_uses must be positive when set")
		return
	}

	code, err := auth.GenerateInviteCode()
	if err != nil {
		internalError(w, err.Error())
		return
	}

	invite, err := s.queries.CreateInvite(r.Context(), db.CreateInviteParams{
		Code: code, Name: req.Name, MaxUses: req.MaxUses, CreatedBy: user.ID,
	})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	slog.Info("admin: invite created", "admin", user.Username, "name", req.Name, "max_uses", req.MaxUses)
	created(w, newInviteResponse(invite))
}

// listInvites lists every invite.
func (s *Server) listInvites(w http.ResponseWriter, r *http.Request) {
	invites, err := s.queries.ListInvites(r.Context())
	if err != nil {
		internalError(w, err.Error())
		return
	}

	resp := make([]inviteResponse, len(invites))
	for i, invite := range invites {
		resp[i] = newInviteResponse(invite)
	}
	ok(w, resp)
}

// deleteInvite revokes an invite.
func (s *Server) deleteInvite(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid id in path")
		return
	}

	rows, err := s.queries.DeleteInvite(r.Context(), id)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	if rows == 0 {
		http.NotFound(w, r)
		return
	}
	slog.Info("admin: invite deleted", "admin", user.Username, "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// reindex kicks off a full Meilisearch rebuild in the background.
func (s *Server) reindex(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	if !s.search.Enabled() {
		badRequest(w, "search is not configured")
		return
	}

	slog.Info("admin: reindex started", "admin", user.Username)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := s.search.Reindex(ctx, s.queries); err != nil {
			slog.Error("admin: reindex failed", "err", err)
			return
		}
		slog.Info("admin: reindex finished")
	}()

	write(w, http.StatusAccepted, map[string]string{"status": "reindex started"})
}

// recomputeStats kicks off a full stats rollup rebuild in the background.
func (s *Server) recomputeStats(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	slog.Info("admin: stats recompute started", "admin", user.Username)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := rollup.RecomputeAll(ctx, s.pool, s.queries); err != nil {
			slog.Error("admin: stats recompute failed", "err", err)
			return
		}
		if err := s.stats.InvalidateAll(ctx); err != nil {
			slog.Error("admin: stats cache invalidate failed", "err", err)
			return
		}
		slog.Info("admin: stats recompute finished")
	}()

	write(w, http.StatusAccepted, map[string]string{"status": "recompute started"})
}
