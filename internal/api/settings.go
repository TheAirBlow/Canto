package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"

	"github.com/jackc/pgx/v5"

	"Canto/internal/auth"
	"Canto/internal/correlate"
	"Canto/internal/db"
	"Canto/internal/source"
)

// registerSettings registers the per-user settings endpoint.
func (s *Server) registerSettings(mux authMux) {
	mux.CookieAuthHandleFunc("GET /settings", s.getSettings)
	mux.CookieAuthHandleFunc("PUT /settings", s.putSettings)
	mux.CookieAuthHandleFunc("GET /settings/registry", s.getSettingsRegistry)
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

// registryProcessor describes one registered link/fallback processor's capabilities and current availability.
type registryProcessor struct {
	ID        string `json:"id"`
	CanDetect bool   `json:"can_detect"`
	CanLookup bool   `json:"can_lookup"`
	Available bool   `json:"available"`
}

// registryMatcher describes one registered fuzzy matcher's current availability.
type registryMatcher struct {
	ID        string `json:"id"`
	Available bool   `json:"available"`
}

// registryIngester describes one ingest endpoint Canto exposes.
type registryIngester struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	APIPath string `json:"api_path"`
}

// settingsRegistryResponse is GET /settings/registry's payload: everything the server actually has registered.
type settingsRegistryResponse struct {
	Processors []registryProcessor `json:"processors"`
	Matchers   []registryMatcher   `json:"matchers"`
	Ingesters  []registryIngester  `json:"ingesters"`
}

// getSettingsRegistry returns every processor, fuzzy matcher, and ingest endpoint the server has registered.
func (s *Server) getSettingsRegistry(w http.ResponseWriter, r *http.Request) {
	processorIDs := s.registry.IDs()
	sort.Strings(processorIDs)
	processors := make([]registryProcessor, 0, len(processorIDs))
	for _, id := range processorIDs {
		p, _ := s.registry.ByID(id)
		state := p.State(r.Context())
		available := state.CanDetect || state.CanLookup
		if a, ok := p.(source.Availabler); ok {
			available = a.Available()
		}
		processors = append(processors, registryProcessor{
			ID: id, CanDetect: state.CanDetect, CanLookup: state.CanLookup, Available: available,
		})
	}

	matcherIDs := s.matchers.IDs()
	sort.Strings(matcherIDs)
	matchers := make([]registryMatcher, 0, len(matcherIDs))
	for _, id := range matcherIDs {
		matcher, _ := s.matchers.ByID(id)
		available := true
		if availabler, ok := matcher.(correlate.Availabler); ok {
			available = availabler.Available()
		}
		matchers = append(matchers, registryMatcher{ID: id, Available: available})
	}

	ok(w, settingsRegistryResponse{Processors: processors, Matchers: matchers, Ingesters: knownIngesters})
}

// resolveSettings loads userID's stored settings in one round trip, falling back to Canto's configured defaults.
func (s *Server) resolveSettings(ctx context.Context, userID int64) (settingsResponse, error) {
	row, err := s.queries.GetUserSettings(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return settingsResponse{
			LinkProcessors:     s.defaults.LinkOrder,
			FallbackProcessors: s.defaults.FallbackOrder,
			FuzzyMatchers:      s.defaults.MatcherOrder,
			Ingesters:          s.ingestDefaults.Enabled,
			Forwards:           []forwardRule{},
		}, nil
	}
	if err != nil {
		return settingsResponse{}, err
	}

	var settings settingsResponse
	if err := json.Unmarshal(row.Settings, &settings); err != nil {
		return settingsResponse{}, err
	}
	if settings.Forwards == nil {
		settings.Forwards = []forwardRule{}
	}
	return settings, nil
}
