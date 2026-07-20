package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"Canto/internal/auth"
	"Canto/internal/db"
)

// registerAPIKeys registers the API key management endpoints.
func (s *Server) registerAPIKeys(mux authMux) {
	mux.CookieAuthHandleFunc("GET /keys", s.listAPIKeys)
	mux.CookieAuthHandleFunc("POST /keys", s.createAPIKey)
	mux.CookieAuthHandleFunc("DELETE /keys/{id}", s.deleteAPIKey)
}

// apiKeyResponse is the public-facing API key shape.
type apiKeyResponse struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// newAPIKeyResponse strips the hash off a db.ApiKey.
func newAPIKeyResponse(k db.ApiKey) apiKeyResponse {
	resp := apiKeyResponse{ID: k.ID, Name: k.Name, CreatedAt: k.CreatedAt.Time}
	if k.LastUsedAt.Valid {
		resp.LastUsedAt = &k.LastUsedAt.Time
	}
	return resp
}

// listAPIKeys lists the caller's API keys.
func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	keys, err := s.queries.ListAPIKeysForUser(r.Context(), user.ID)
	if err != nil {
		internalError(w, err.Error())
		return
	}

	resp := make([]apiKeyResponse, len(keys))
	for i, k := range keys {
		resp[i] = newAPIKeyResponse(k)
	}
	ok(w, resp)
}

// createAPIKeyRequest is the JSON body for creating an API key.
type createAPIKeyRequest struct {
	Name string `json:"name"`
}

// createAPIKeyResponse includes the raw key in apiKeyResponse.
type createAPIKeyResponse struct {
	apiKeyResponse
	Key string `json:"key"`
}

// createAPIKey mints a new UUID API key for the caller.
func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	var req createAPIKeyRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(w, "name is required")
		return
	}

	key, err := auth.GenerateAPIKey()
	if err != nil {
		internalError(w, err.Error())
		return
	}

	row, err := s.queries.CreateAPIKey(r.Context(), db.CreateAPIKeyParams{
		UserID: user.ID, Name: req.Name, KeyHash: auth.HashToken(key),
	})
	if err != nil {
		internalError(w, err.Error())
		return
	}

	created(w, createAPIKeyResponse{apiKeyResponse: newAPIKeyResponse(row), Key: key})
}

// deleteAPIKey revokes one of the caller's API keys.
func (s *Server) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid id in path")
		return
	}

	rows, err := s.queries.DeleteAPIKey(r.Context(), db.DeleteAPIKeyParams{ID: id, UserID: user.ID})
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
