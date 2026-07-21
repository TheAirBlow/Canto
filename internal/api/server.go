package api

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"Canto/internal/config"
	"Canto/internal/db"
	"Canto/internal/export"
	"Canto/internal/importer"
	"Canto/internal/ingest"
	"Canto/internal/search"
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
	api.HandleFunc("GET /health", s.health)
	mux.Handle("/api/", http.StripPrefix("/api", api))
}

// health returns a successful health report.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]string{"status": "ok"})
}
