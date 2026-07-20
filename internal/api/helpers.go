package api

import (
	"Canto/internal/auth"
	"Canto/internal/db"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// imageURL builds a "medium" size image URL for id, or nil if id is unset.
func imageURL(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	url := fmt.Sprintf("/images/%s/medium", uuid.UUID(id.Bytes).String())
	return &url
}

// ok sends an HTTP 200 response with value JSON-encoded.
func ok(w http.ResponseWriter, value any) {
	write(w, http.StatusOK, value)
}

// created sends an HTTP 201 response with value JSON-encoded.
func created(w http.ResponseWriter, value any) {
	write(w, http.StatusCreated, value)
}

// badRequest sends an HTTP 400 error response.
func badRequest(w http.ResponseWriter, detail string) {
	fail(w, http.StatusBadRequest, "bad request", detail)
}

// unauthorized sends an HTTP 401 error response.
func unauthorized(w http.ResponseWriter, detail string) {
	fail(w, http.StatusUnauthorized, "unauthorized", detail)
}

// conflict sends an HTTP 409 error response.
func conflict(w http.ResponseWriter, detail string) {
	fail(w, http.StatusConflict, "conflict", detail)
}

// forbidden sends an HTTP 403 error response.
func forbidden(w http.ResponseWriter, detail string) {
	fail(w, http.StatusForbidden, "forbidden", detail)
}

// internalError sends an HTTP 500 error response.
func internalError(w http.ResponseWriter, detail string) {
	fail(w, http.StatusInternalServerError, "internal error", detail)
}

// fail sends a JSON error response with the given status, title, and detail.
func fail(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"title": title, "detail": detail})
}

// write sends value JSON-encoded with the given status.
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// decode JSON-decodes the request body into v.
func decode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// errMissingSessionCookie/errInvalidSession/errSessionExpired are the auth-layer failures authenticateCookie reports as 401s.
var (
	errMissingSessionCookie = errors.New("missing session cookie")
	errInvalidSession       = errors.New("invalid session")
	errSessionExpired       = errors.New("session expired")
)

// isAuthError reports whether err is one of authenticateCookie's expected 401 cases.
func isAuthError(err error) bool {
	return errors.Is(err, errMissingSessionCookie) || errors.Is(err, errInvalidSession) || errors.Is(err, errSessionExpired)
}

// authenticateCookie resolves r's session cookie into a user.
func (s *Server) authenticateCookie(r *http.Request) (db.User, error) {
	cookie, err := r.Cookie(s.auth.CookieName)
	if err != nil {
		return db.User{}, errMissingSessionCookie
	}

	row, err := s.queries.GetSessionUser(r.Context(), auth.HashToken(cookie.Value))
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, errInvalidSession
	}
	if err != nil {
		return db.User{}, err
	}
	if row.SessionExpiresAt.Time.Before(time.Now()) {
		return db.User{}, errSessionExpired
	}

	return db.User{
		ID:           row.ID,
		Username:     row.Username,
		PasswordHash: row.PasswordHash,
		PublicStats:  row.PublicStats,
		IsAdmin:      row.IsAdmin,
		CreatedAt:    row.CreatedAt,
	}, nil
}

// authMux extends http.ServeMux with a CookieAuthHandleFunc registration method.
type authMux struct {
	*http.ServeMux
	server Server
}

// CookieAuthHandleFunc registers handler on pattern, wrapped with RequireAuth.
func (m authMux) CookieAuthHandleFunc(pattern string, handler http.HandlerFunc) {
	m.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := m.server.authenticateCookie(r)
		if err != nil {
			if isAuthError(err) {
				unauthorized(w, err.Error())
			} else {
				internalError(w, err.Error())
			}
			return
		}
		handler.ServeHTTP(w, r.WithContext(auth.ContextWithUser(r.Context(), user)))
	}))
}

// AdminAuthHandleFunc registers handler on pattern, wrapped with RequireAuth plus an is_admin check.
func (m authMux) AdminAuthHandleFunc(pattern string, handler http.HandlerFunc) {
	m.CookieAuthHandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		user, _ := auth.UserFromContext(r.Context())
		if !user.IsAdmin {
			forbidden(w, "admin access required")
			return
		}
		handler(w, r)
	})
}

// TokenAuthHandleFunc registers handler on pattern, wrapped with API-key authentication.
func (m authMux) TokenAuthHandleFunc(pattern string, handler http.HandlerFunc) {
	m.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Token ")
		if token == "" {
			unauthorized(w, "missing Authorization token")
			return
		}

		keyHash := auth.HashToken(token)
		user, err := m.server.queries.GetUserByAPIKeyHash(r.Context(), keyHash)
		if errors.Is(err, pgx.ErrNoRows) {
			unauthorized(w, "invalid token")
			return
		}
		if err != nil {
			internalError(w, err.Error())
			return
		}
		if err := m.server.queries.TouchAPIKey(r.Context(), keyHash); err != nil {
			slog.Warn("touch api key failed", "err", err)
		}

		handler.ServeHTTP(w, r.WithContext(auth.ContextWithUser(r.Context(), user)))
	}))
}
