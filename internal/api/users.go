package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/auth"
	"Canto/internal/db"
	"Canto/internal/images"
	"Canto/internal/search"
)

const (
	usersPageDefault = 50
	usersPageMax     = 200
)

// registerUsers registers the auth and user profile endpoints.
func (s *Server) registerUsers(mux authMux) {
	mux.HandleFunc("POST /user/register", s.register)
	mux.HandleFunc("POST /user/login", s.login)
	mux.CookieAuthHandleFunc("POST /user/logout", s.logout)
	mux.OptionalAuthHandleFunc("GET /user", s.listUsers)
	mux.OptionalAuthHandleFunc("GET /user/{id}", s.getUser)
	mux.CookieAuthHandleFunc("GET /user/me", s.me)
	mux.CookieAuthHandleFunc("PUT /user/me", s.updateProfile)
	mux.CookieAuthHandleFunc("PUT /user/me/image", s.uploadProfileImage)
	mux.CookieAuthHandleFunc("PUT /user/me/credentials", s.updateCredentials)
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
	DisplayName *string   `json:"display_name,omitempty"`
	Description *string   `json:"description,omitempty"`
	ImageURL    *string   `json:"image_url,omitempty"`
	Public      bool      `json:"public"`
	IsAdmin     bool      `json:"is_admin"`
	CreatedAt   time.Time `json:"created_at"`
}

// newUserResponse strips sensitive fields off a db.User.
func newUserResponse(u db.User) userResponse {
	return userResponse{
		ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Description: u.Description,
		ImageURL: imageURL(u.ImageID), Public: u.Public, IsAdmin: u.IsAdmin, CreatedAt: u.CreatedAt.Time,
	}
}

// register creates a new user account.
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	username, err := auth.ValidateUsername(req.Username)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	req.Username = username
	if len(req.Password) < 8 {
		badRequest(w, "password must be at least 8 characters")
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

	user, err := s.queries.GetUserByUsername(r.Context(), strings.ToLower(strings.TrimSpace(req.Username)))
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

// registerWithInvite claims code and creates username's account in one transaction, so a failed registration never burns an invite use.
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

// resolveUserPath resolves an id-or-username path segment into a db.User.
func (s *Server) resolveUserPath(ctx context.Context, raw string) (db.User, error) {
	if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return s.queries.GetUserByID(ctx, id)
	}
	username, err := auth.ValidateUsername(raw)
	if err != nil {
		return db.User{}, pgx.ErrNoRows
	}
	return s.queries.GetUserByUsername(ctx, username)
}

// listUsers lists public user profiles, paginated.
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	var after int64
	if raw := r.URL.Query().Get("after"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			badRequest(w, "invalid after")
			return
		}
		after = v
	}
	limit := usersPageDefault
	if raw := r.URL.Query().Get("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			badRequest(w, "invalid limit")
			return
		}
		limit = v
	}
	if limit > usersPageMax {
		limit = usersPageMax
	}

	rows, err := s.queries.ListPublicUsers(r.Context(), db.ListPublicUsersParams{After: after, MaxRows: int32(limit)})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	resp := make([]userResponse, len(rows))
	for i, row := range rows {
		resp[i] = newUserResponse(row)
	}
	ok(w, resp)
}

// getUser returns a single user profile by id or username. A private profile 404s unless it's the caller's own.
func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	caller, _ := auth.UserFromContext(r.Context())

	target, err := s.resolveUserPath(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !target.Public && target.ID != caller.ID {
		http.NotFound(w, r)
		return
	}
	ok(w, newUserResponse(target))
}

// updateProfileRequest is the JSON body for editing the caller's own profile.
type updateProfileRequest struct {
	DisplayName *string `json:"display_name"`
	Description *string `json:"description"`
	Public      bool    `json:"public"`
}

// updateProfile edits the caller's own display name, description, and profile visibility.
func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	caller, _ := auth.UserFromContext(r.Context())

	var req updateProfileRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}

	user, err := s.queries.UpdateUserProfile(r.Context(), db.UpdateUserProfileParams{
		ID: caller.ID, DisplayName: req.DisplayName, Description: req.Description, Public: req.Public,
	})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	s.syncUserSearchIndex(r.Context(), user)
	ok(w, newUserResponse(user))
}

// uploadProfileImage replaces the caller's own avatar from a multipart upload.
func (s *Server) uploadProfileImage(w http.ResponseWriter, r *http.Request) {
	caller, _ := auth.UserFromContext(r.Context())

	imageID, err := storeUploadedImage(r)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	user, err := s.queries.SetUserImage(r.Context(), db.SetUserImageParams{ID: caller.ID, ImageID: uuidParam(imageID)})
	if err != nil {
		internalError(w, err.Error())
		return
	}
	images.DeleteIfSet(caller.ImageID)
	s.syncUserSearchIndex(r.Context(), user)
	ok(w, newUserResponse(user))
}

// updateCredentialsRequest is the JSON body for changing the caller's own username and/or password.
type updateCredentialsRequest struct {
	CurrentPassword string  `json:"current_password"`
	NewUsername     *string `json:"new_username,omitempty"`
	NewPassword     *string `json:"new_password,omitempty"`
}

// updateCredentials changes the caller's own username and/or password behind a current-password check, dropping every other session on a password change.
func (s *Server) updateCredentials(w http.ResponseWriter, r *http.Request) {
	caller, _ := auth.UserFromContext(r.Context())

	var req updateCredentialsRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	if req.NewUsername == nil && req.NewPassword == nil {
		badRequest(w, "new_username or new_password is required")
		return
	}
	if !auth.VerifyPassword(caller.PasswordHash, req.CurrentPassword) {
		unauthorized(w, "current password is incorrect")
		return
	}

	user := caller
	if req.NewUsername != nil {
		username, err := auth.ValidateUsername(*req.NewUsername)
		if err != nil {
			badRequest(w, err.Error())
			return
		}
		var pgErr *pgconn.PgError
		user, err = s.queries.UpdateUserUsername(r.Context(), db.UpdateUserUsernameParams{ID: caller.ID, Username: username})
		switch {
		case errors.As(err, &pgErr) && pgErr.Code == "23505":
			conflict(w, "username already taken")
			return
		case err != nil:
			internalError(w, err.Error())
			return
		}
	}

	if req.NewPassword != nil {
		if len(*req.NewPassword) < 8 {
			badRequest(w, "password must be at least 8 characters")
			return
		}
		hash, err := auth.HashPassword(*req.NewPassword)
		if err != nil {
			internalError(w, err.Error())
			return
		}
		user, err = s.queries.UpdateUserPassword(r.Context(), db.UpdateUserPasswordParams{ID: caller.ID, PasswordHash: hash})
		if err != nil {
			internalError(w, err.Error())
			return
		}
		if cookie, err := r.Cookie(s.auth.CookieName); err == nil {
			if err := s.queries.DeleteOtherSessionsForUser(r.Context(), db.DeleteOtherSessionsForUserParams{
				UserID: caller.ID, TokenHash: auth.HashToken(cookie.Value),
			}); err != nil {
				slog.Error("delete other sessions failed", "err", err)
			}
		}
	}

	s.syncUserSearchIndex(r.Context(), user)
	ok(w, newUserResponse(user))
}

// syncUserSearchIndex keeps the "users" search index matching user's current visibility.
func (s *Server) syncUserSearchIndex(ctx context.Context, user db.User) {
	if user.Public {
		s.search.Upsert(ctx, "users", search.UserDocument(user))
	} else {
		s.search.Delete(ctx, "users", user.ID)
	}
}
