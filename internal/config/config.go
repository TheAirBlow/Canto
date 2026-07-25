package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is the fully-resolved runtime configuration.
type Config struct {
	Server      ServerConfig      `json:"server"`
	Database    DatabaseConfig    `json:"database"`
	Meilisearch MeilisearchConfig `json:"meilisearch"`
	Providers   ProvidersConfig   `json:"providers"`
	Import      ImportConfig      `json:"import"`
	Stats       StatsConfig       `json:"stats"`
	Auth        AuthConfig        `json:"auth"`
	Processors  ProcessorsConfig  `json:"processors"`
	Ingest      IngestConfig      `json:"ingest"`
	Refresh     RefreshConfig     `json:"refresh"`
	Rollup      RollupConfig      `json:"rollup"`
	Correlation CorrelationConfig `json:"correlation"`
}

// CorrelationConfig controls fuzzy-match candidate retrieval, scoring weights, and decision thresholds.
type CorrelationConfig struct {
	CandidateLimit    int     `json:"candidate_limit"`
	ReconcilerWorkers int     `json:"reconciler_workers"`
	PreferChinese     bool    `json:"prefer_chinese"`
	NameWeight        float64 `json:"name_weight"`
	ArtistWeight      float64 `json:"artist_weight"`
	DurationWeight    float64 `json:"duration_weight"`
	TrackWeight       float64 `json:"track_weight"`
	AmbiguityWeight   float64 `json:"ambiguity_weight"`
	GapFloor          float64 `json:"gap_floor"`
	AutoAccept        float64 `json:"auto_accept"`
	SuggestMin        float64 `json:"suggest_min"`
	DurationVetoMs    int32   `json:"duration_veto_ms"`
}

// RollupConfig controls the stats rollup writer's batching cadence.
type RollupConfig struct {
	FlushInterval time.Duration `json:"flush_interval"`
}

// RefreshConfig controls the background metadata-refresh worker.
type RefreshConfig struct {
	Interval time.Duration `json:"interval"`
	Entities []string      `json:"entities"`
}

// ProcessorsConfig holds the default source-processor and fuzzy-matcher ordering.
type ProcessorsConfig struct {
	LinkOrder     []string `json:"link_order"`
	FallbackOrder []string `json:"fallback_order"`
	MatcherOrder  []string `json:"matcher_order"`
}

// IngestConfig holds the default enabled ingest endpoints.
type IngestConfig struct {
	Enabled []string `json:"enabled"`
}

// ServerConfig controls the HTTP listener.
type ServerConfig struct {
	BindAddr string `json:"bind_addr"`
	Port     int    `json:"port"`
	Debug    bool   `json:"debug"`
	Trace    bool   `json:"trace"`
}

// AuthConfig controls session cookies and password hashing.
type AuthConfig struct {
	SessionTTL       time.Duration `json:"session_ttl"`
	CookieName       string        `json:"cookie_name"`
	CookieSecure     bool          `json:"cookie_secure"`
	RegistrationMode string        `json:"registration_mode"` // "public" or "invite_only"
}

// DatabaseConfig points at the Postgres instance.
type DatabaseConfig struct {
	URL string `json:"url"`
}

// MeilisearchConfig points at the Meilisearch instance used for fuzzy correlation and search.
type MeilisearchConfig struct {
	URL    string `json:"url"`
	APIKey string `json:"api_key"`
}

// ProvidersConfig toggles/configures external metadata providers.
type ProvidersConfig struct {
	MusicBrainzURL       string `json:"musicbrainz_url"`
	MusicBrainzRateLimit int    `json:"musicbrainz_rate_limit"`
	LastFMAPIKey         string `json:"lastfm_api_key"`
	LastFMAPISecret      string `json:"lastfm_api_secret"`
}

// ImportConfig controls the bulk-import worker pool.
type ImportConfig struct {
	Workers       int `json:"workers"`
	EnrichWorkers int `json:"enrich_workers"`
}

// StatsConfig controls the stats_cache regeneration cadence.
type StatsConfig struct {
	RegenInterval time.Duration `json:"regen_interval"`
}

// DataDir is where the data volume is mounted.
const DataDir = "./data"

// defaults returns the hard-coded fallback configuration.
func defaults() Config {
	return Config{
		Server: ServerConfig{
			BindAddr: "0.0.0.0",
			Port:     8080,
		},
		Database: DatabaseConfig{
			URL: "postgres://canto:canto@127.0.0.1:5432/canto?sslmode=disable",
		},
		Meilisearch: MeilisearchConfig{
			URL: "http://localhost:7700",
		},
		Providers: ProvidersConfig{
			MusicBrainzURL:       "https://musicbrainz.org/ws/2",
			MusicBrainzRateLimit: 1,
		},
		Import: ImportConfig{
			Workers:       32,
			EnrichWorkers: 32,
		},
		Stats: StatsConfig{
			RegenInterval: 5 * time.Minute,
		},
		Auth: AuthConfig{
			SessionTTL:       30 * 24 * time.Hour,
			CookieName:       "canto_session",
			CookieSecure:     true,
			RegistrationMode: "invite_only",
		},
		Processors: ProcessorsConfig{
			LinkOrder:     []string{"ytmusic", "spotify", "deezer", "musicbrainz"},
			FallbackOrder: []string{"musicbrainz", "spotify", "lastfm", "deezer"},
			MatcherOrder:  []string{"exact", "trigram", "meilisearch"},
		},
		Ingest: IngestConfig{
			Enabled: []string{"listenbrainz"},
		},
		Refresh: RefreshConfig{
			Interval: 7 * 24 * time.Hour,
			Entities: []string{"artist"},
		},
		Rollup: RollupConfig{
			FlushInterval: time.Second,
		},
		Correlation: CorrelationConfig{
			CandidateLimit:    10,
			ReconcilerWorkers: 4,
			PreferChinese:     false,
			NameWeight:        0.6,
			ArtistWeight:      0.3,
			DurationWeight:    0.1,
			TrackWeight:       0.3,
			AmbiguityWeight:   0.5,
			GapFloor:          0.15,
			AutoAccept:        0.82,
			SuggestMin:        0.6,
			DurationVetoMs:    15000,
		},
	}
}

// Load loads the Canto Config either from a JSON file or from env variables.
func Load(dataDir string) (Config, error) {
	cfg := defaults()

	configFile := envString("CANTO_CONFIG_FILE", filepath.Join(dataDir, "config.json"))
	if data, err := os.ReadFile(configFile); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("config: parse %s: %w", configFile, err)
		}
	} else if !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("config: read %s: %w", configFile, err)
	}

	cfg.Server.BindAddr = envString("CANTO_SERVER_BIND_ADDR", cfg.Server.BindAddr)
	cfg.Server.Port = envInt("CANTO_SERVER_PORT", cfg.Server.Port)
	cfg.Server.Debug = envBool("CANTO_SERVER_DEBUG", cfg.Server.Debug)
	cfg.Server.Trace = envBool("CANTO_SERVER_TRACE", cfg.Server.Trace)
	cfg.Database.URL = envString("CANTO_DATABASE_URL", cfg.Database.URL)
	cfg.Meilisearch.URL = envString("CANTO_MEILISEARCH_URL", cfg.Meilisearch.URL)
	cfg.Meilisearch.APIKey = envString("CANTO_MEILISEARCH_API_KEY", cfg.Meilisearch.APIKey)
	cfg.Providers.MusicBrainzURL = envString("CANTO_PROVIDERS_MUSICBRAINZ_URL", cfg.Providers.MusicBrainzURL)
	cfg.Providers.MusicBrainzRateLimit = envInt("CANTO_PROVIDERS_MUSICBRAINZ_RATE_LIMIT", cfg.Providers.MusicBrainzRateLimit)
	cfg.Providers.LastFMAPIKey = envString("CANTO_PROVIDERS_LASTFM_API_KEY", cfg.Providers.LastFMAPIKey)
	cfg.Providers.LastFMAPISecret = envString("CANTO_PROVIDERS_LASTFM_API_SECRET", cfg.Providers.LastFMAPISecret)
	cfg.Import.Workers = envInt("CANTO_IMPORT_WORKERS", cfg.Import.Workers)
	cfg.Import.EnrichWorkers = envInt("CANTO_IMPORT_ENRICH_WORKERS", cfg.Import.EnrichWorkers)
	cfg.Stats.RegenInterval = envDuration("CANTO_STATS_REGEN_INTERVAL", cfg.Stats.RegenInterval)
	cfg.Auth.SessionTTL = envDuration("CANTO_AUTH_SESSION_TTL", cfg.Auth.SessionTTL)
	cfg.Auth.CookieName = envString("CANTO_AUTH_COOKIE_NAME", cfg.Auth.CookieName)
	cfg.Auth.CookieSecure = envBool("CANTO_AUTH_COOKIE_SECURE", cfg.Auth.CookieSecure)
	cfg.Auth.RegistrationMode = envString("CANTO_AUTH_REGISTRATION_MODE", cfg.Auth.RegistrationMode)
	cfg.Processors.LinkOrder = envStringSlice("CANTO_PROCESSORS_LINK_ORDER", cfg.Processors.LinkOrder)
	cfg.Processors.FallbackOrder = envStringSlice("CANTO_PROCESSORS_FALLBACK_ORDER", cfg.Processors.FallbackOrder)
	cfg.Processors.MatcherOrder = envStringSlice("CANTO_PROCESSORS_MATCHER_ORDER", cfg.Processors.MatcherOrder)
	cfg.Ingest.Enabled = envStringSlice("CANTO_INGEST_ENABLED", cfg.Ingest.Enabled)
	cfg.Refresh.Interval = envDuration("CANTO_REFRESH_INTERVAL", cfg.Refresh.Interval)
	cfg.Refresh.Entities = envStringSlice("CANTO_REFRESH_ENTITIES", cfg.Refresh.Entities)
	cfg.Rollup.FlushInterval = envDuration("CANTO_ROLLUP_FLUSH_INTERVAL", cfg.Rollup.FlushInterval)
	cfg.Correlation.CandidateLimit = envInt("CANTO_CORRELATION_CANDIDATE_LIMIT", cfg.Correlation.CandidateLimit)
	cfg.Correlation.ReconcilerWorkers = envInt("CANTO_CORRELATION_RECONCILER_WORKERS", cfg.Correlation.ReconcilerWorkers)
	cfg.Correlation.PreferChinese = envBool("CANTO_CORRELATION_PREFER_CHINESE", cfg.Correlation.PreferChinese)
	cfg.Correlation.NameWeight = envFloat("CANTO_CORRELATION_NAME_WEIGHT", cfg.Correlation.NameWeight)
	cfg.Correlation.ArtistWeight = envFloat("CANTO_CORRELATION_ARTIST_WEIGHT", cfg.Correlation.ArtistWeight)
	cfg.Correlation.DurationWeight = envFloat("CANTO_CORRELATION_DURATION_WEIGHT", cfg.Correlation.DurationWeight)
	cfg.Correlation.TrackWeight = envFloat("CANTO_CORRELATION_TRACK_WEIGHT", cfg.Correlation.TrackWeight)
	cfg.Correlation.AmbiguityWeight = envFloat("CANTO_CORRELATION_AMBIGUITY_WEIGHT", cfg.Correlation.AmbiguityWeight)
	cfg.Correlation.GapFloor = envFloat("CANTO_CORRELATION_GAP_FLOOR", cfg.Correlation.GapFloor)
	cfg.Correlation.AutoAccept = envFloat("CANTO_CORRELATION_AUTO_ACCEPT", cfg.Correlation.AutoAccept)
	cfg.Correlation.SuggestMin = envFloat("CANTO_CORRELATION_SUGGEST_MIN", cfg.Correlation.SuggestMin)
	cfg.Correlation.DurationVetoMs = int32(envInt("CANTO_CORRELATION_DURATION_VETO_MS", int(cfg.Correlation.DurationVetoMs)))

	return cfg, nil
}

// envString reads key falling back to def.
func envString(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	if path, ok := os.LookupEnv(key + "_FILE"); ok {
		if data, err := os.ReadFile(path); err == nil {
			return string(data)
		}
	}
	return def
}

// envInt reads key as an int, falling back to def on error or absence.
func envInt(key string, def int) int {
	if v := envString(key, ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// envFloat reads key as a float64, falling back to def on error or absence.
func envFloat(key string, def float64) float64 {
	if v := envString(key, ""); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// envBool reads key as a bool, falling back to def on error or absence.
func envBool(key string, def bool) bool {
	if v := envString(key, ""); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

// envStringSlice reads key as a comma-separated list, falling back to def if unset.
func envStringSlice(key string, def []string) []string {
	v := envString(key, "")
	if v == "" {
		return def
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// envDuration reads key as a Go duration string, falling back to def on error or absence.
func envDuration(key string, def time.Duration) time.Duration {
	if v := envString(key, ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
