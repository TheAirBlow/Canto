package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"Canto/internal/auth"
	"Canto/internal/db"
)

// registerSettings registers the per-user settings endpoint.
func (s *Server) registerSettings(mux authMux) {
	mux.CookieAuthHandleFunc("GET /settings", s.getSettings)
	mux.CookieAuthHandleFunc("PUT /settings", s.putSettings)
}

// forwardRule represents an ingester forwarding rule.
type forwardRule struct {
	Ingester string `json:"ingester"`
	Type     string `json:"type"`
	URL      string `json:"url"`
	Token    string `json:"token"`
}

// settingsResponse is the per-user configuration shape.
type settingsResponse struct {
	LinkProcessors     []string      `json:"link_processors"`
	FallbackProcessors []string      `json:"fallback_processors"`
	FuzzyMatchers      []string      `json:"fuzzy_matchers"`
	FuzzyNormalize     bool          `json:"fuzzy_normalize"`
	Ingesters          []string      `json:"ingesters"`
	Forwards           []forwardRule `json:"forwards"`
}

// getSettings returns the caller's settings, or Canto's defaults if they haven't customized them.
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	settings, err := s.resolveSettings(r.Context(), user.ID)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, settings)
}

// putSettings replaces the caller's settings.
func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	var req settingsResponse
	if err := decode(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}

	settingsJSON, err := json.Marshal(req)
	if err != nil {
		badRequest(w, err.Error())
		return
	}

	if _, err := s.queries.UpsertUserSettings(r.Context(), db.UpsertUserSettingsParams{
		UserID: user.ID, Settings: settingsJSON,
	}); err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, req)
}

// resolveSettings loads userID's stored settings in one round trip, falling back to Canto's configured defaults.
func (s *Server) resolveSettings(ctx context.Context, userID int64) (settingsResponse, error) {
	row, err := s.queries.GetUserSettings(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return settingsResponse{
			LinkProcessors:     s.defaults.LinkOrder,
			FallbackProcessors: s.defaults.FallbackOrder,
			FuzzyMatchers:      s.defaults.MatcherOrder,
			FuzzyNormalize:     s.defaults.Normalize,
			Ingesters:          s.ingestDefaults.Enabled,
		}, nil
	}
	if err != nil {
		return settingsResponse{}, err
	}

	var settings settingsResponse
	if err := json.Unmarshal(row.Settings, &settings); err != nil {
		return settingsResponse{}, err
	}
	return settings, nil
}
