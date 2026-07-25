package api

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"Canto/internal/config"
	"Canto/internal/correlate"
	"Canto/internal/db"
	"Canto/internal/export"
	"Canto/internal/importer"
	"Canto/internal/ingest"
	"Canto/internal/rollup"
	"Canto/internal/search"
	"Canto/internal/source"
	"Canto/internal/stats"
)

// Deps holds every dependency Server's handlers need.
type Deps struct {
	Queries        *db.Queries
	Pool           *pgxpool.Pool
	Auth           config.AuthConfig
	Defaults       config.ProcessorsConfig
	IngestDefaults config.IngestConfig
	Providers      config.ProvidersConfig
	Ingest         *ingest.Service
	Search         *search.Client
	Importer       *importer.Manager
	Export         *export.Service
	Stats          *stats.Engine
	Registry       *source.Registry
	Matchers       *correlate.MatcherRegistry
	Unrecorder     *rollup.Unrecorder
}

// Server holds the dependencies every REST handler needs.
type Server struct {
	queries        *db.Queries
	pool           *pgxpool.Pool
	auth           config.AuthConfig
	defaults       config.ProcessorsConfig
	ingestDefaults config.IngestConfig
	providers      config.ProvidersConfig
	ingest         *ingest.Service
	search         *search.Client
	importer       *importer.Manager
	export         *export.Service
	stats          *stats.Engine
	registry       *source.Registry
	matchers       *correlate.MatcherRegistry
	unrecorder     *rollup.Unrecorder
}

// NewServer builds a Server from deps.
func NewServer(deps Deps) *Server {
	return &Server{
		queries:        deps.Queries,
		pool:           deps.Pool,
		auth:           deps.Auth,
		defaults:       deps.Defaults,
		ingestDefaults: deps.IngestDefaults,
		providers:      deps.Providers,
		ingest:         deps.Ingest,
		search:         deps.Search,
		importer:       deps.Importer,
		export:         deps.Export,
		stats:          deps.Stats,
		registry:       deps.Registry,
		matchers:       deps.Matchers,
		unrecorder:     deps.Unrecorder,
	}
}

// Register mounts every Canto API route onto mux.
func (s *Server) Register(mux *http.ServeMux) {
	api := authMux{ServeMux: http.NewServeMux(), server: *s}
	s.registerUsers(api)
	s.registerAPIKeys(api)
	s.registerSettings(api)
	s.registerListenBrainz(api)
	s.registerArtists(api)
	s.registerAlbums(api)
	s.registerSongs(api)
	s.registerAdmin(api)
	s.registerSearch(api)
	s.registerExport(api)
	s.registerImport(api)
	s.registerImages(api)
	s.registerStats(api)
	s.registerListens(api)
	api.HandleFunc("GET /health", s.health)
	mux.Handle("/api/", http.StripPrefix("/api", api))
}

// health returns a successful health report.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]string{"status": "ok"})
}
