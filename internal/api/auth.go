package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/auth"
	"Canto/internal/db"
)

// registerUser registers the auth endpoints.
func (s *Server) registerUser(mux authMux) {
	mux.HandleFunc("POST /auth/register", s.register)
	mux.HandleFunc("POST /auth/login", s.login)
	mux.CookieAuthHandleFunc("POST /auth/logout", s.logout)
	mux.CookieAuthHandleFunc("GET /auth/me", s.me)
}

// credentialsRequest is the JSON body for register and login.
type credentialsRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	InviteCode string `json:"invite_code"`
}

// userResponse is the public-facing user shape.
type userResponse struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	PublicStats bool      `json:"public_stats"`
	IsAdmin     bool      `json:"is_admin"`
	CreatedAt   time.Time `json:"created_at"`
}

// newUserResponse strips sensitive fields off a db.User.
func newUserResponse(u db.User) userResponse {
	return userResponse{ID: u.ID, Username: u.Username, PublicStats: u.PublicStats, IsAdmin: u.IsAdmin, CreatedAt: u.CreatedAt.Time}
}

// register creates a new user account.
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 8 {
		badRequest(w, "username required, password must be at least 8 characters")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		internalError(w, err.Error())
		return
	}

	var user db.User
	if s.auth.RegistrationMode == "invite_only" {
		user, err = s.registerWithInvite(r.Context(), req.Username, hash, strings.TrimSpace(req.InviteCode))
	} else {
		user, err = s.queries.CreateUser(r.Context(), db.CreateUserParams{Username: req.Username, PasswordHash: hash})
	}
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, errInvalidInvite):
		badRequest(w, "invalid or exhausted invite code")
		return
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		conflict(w, "username already taken")
		return
	case err != nil:
		internalError(w, err.Error())
		return
	}
	slog.Info("user registered", "username", user.Username, "invite_only", s.auth.RegistrationMode == "invite_only")

	if err := s.startSession(w, r, user); err != nil {
		internalError(w, err.Error())
		return
	}
	created(w, newUserResponse(user))
}

// login verifies credentials and starts a session.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}

	user, err := s.queries.GetUserByUsername(r.Context(), strings.TrimSpace(req.Username))
	if errors.Is(err, pgx.ErrNoRows) {
		unauthorized(w, "invalid username or password")
		return
	}
	if err != nil {
		internalError(w, err.Error())
		return
	}
	if !auth.VerifyPassword(user.PasswordHash, req.Password) {
		unauthorized(w, "invalid username or password")
		return
	}

	if err := s.startSession(w, r, user); err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, newUserResponse(user))
}

// logout deletes the caller's session and clears the cookie.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(s.auth.CookieName); err == nil {
		if err := s.queries.DeleteSession(r.Context(), auth.HashToken(cookie.Value)); err != nil {
			slog.Error("delete session failed", "err", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: s.auth.CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.auth.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// me returns the currently authenticated user.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		unauthorized(w, "not authenticated")
		return
	}
	write(w, http.StatusOK, newUserResponse(user))
}

// startSession creates a session row and sets the session cookie for user.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, user db.User) error {
	token, err := auth.GenerateSessionToken()
	if err != nil {
		return err
	}

	expiresAt := time.Now().Add(s.auth.SessionTTL)
	_, err = s.queries.CreateSession(r.Context(), db.CreateSessionParams{
		UserID: user.ID, TokenHash: auth.HashToken(token),
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name: s.auth.CookieName, Value: token, Path: "/", Expires: expiresAt,
		HttpOnly: true, Secure: s.auth.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// errInvalidInvite means the supplied invite code is unknown, exhausted, or missing.
var errInvalidInvite = errors.New("invalid invite code")

// registerWithInvite claims code and creates username's account in one transaction, so a failed
// registration never burns an invite use.
func (s *Server) registerWithInvite(ctx context.Context, username, passwordHash, code string) (db.User, error) {
	if code == "" {
		return db.User{}, errInvalidInvite
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.User{}, err
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)

	if _, err := q.ClaimInvite(ctx, code); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.User{}, errInvalidInvite
		}
		return db.User{}, err
	}

	user, err := q.CreateUser(ctx, db.CreateUserParams{Username: username, PasswordHash: passwordHash})
	if err != nil {
		return db.User{}, err
	}
	return user, tx.Commit(ctx)
}
