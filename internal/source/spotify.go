package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"Canto/internal/db"
)

// spotifyIDPattern matches a Spotify base62 entity id.
var spotifyIDPattern = regexp.MustCompile(`^[0-9A-Za-z]{22}$`)

// spotifyNextDataPattern extracts the embed page's __NEXT_DATA__ script body.
var spotifyNextDataPattern = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`)

// spotifyTokenBootstrapID is a public track id used to bootstrap an anonymous pathfinder session token.
const spotifyTokenBootstrapID = "4uLU6hMCjMI75M1A2tKUQC"

// spotifyTokenSkew renews the cached anonymous token this long before its reported expiry.
const spotifyTokenSkew = 60 * time.Second

// spotifyPathfinderURL is Spotify's internal web-player GraphQL endpoint (persisted queries only).
const spotifyPathfinderURL = "https://api-partner.spotify.com/pathfinder/v1/query"

// spotifyAlbumTrackLimit is the tracksV2 page size for an album fetch, large enough for any real album.
const spotifyAlbumTrackLimit = 500

// spotifyOperation is one pathfinder persisted-query's name+hash pair.
type spotifyOperation struct {
	name string
	hash string
}

// Pathfinder operations: name+persisted-query-hash pairs for entity lookups and anonymous search.
var (
	spotifySearchOp       = spotifyOperation{"searchDesktop", "eff59fa0a3d026b88b56fddbcf4bdfa16a186b8175a5c1a358c072e053c2e5b0"}
	spotifyGetTrackOp     = spotifyOperation{"getTrack", "612585ae06ba435ad26369870deaae23b5c8800a256cd8a57e08eddc25a37294"}
	spotifyGetAlbumOp     = spotifyOperation{"getAlbum", "b9bfabef66ed756e5e13f68a942deb60bd4125ec1f1be8cc42769dc0259b4b10"}
	spotifyArtistOverview = spotifyOperation{"queryArtistOverview", "ae0e2958a4ab645b35ca19ac04d0495ae12d9c5d7b7286217674801a9aab281a"}
)

// spotifyProcessor resolves track/album/artist metadata via Spotify's anonymous pathfinder GraphQL API.
type spotifyProcessor struct {
	httpClient *http.Client

	mu             sync.Mutex
	token          string
	tokenExpiresAt time.Time
}

// NewSpotifyProcessor builds the processor.
func NewSpotifyProcessor() Processor {
	return &spotifyProcessor{httpClient: &http.Client{Timeout: 10 * time.Second}}
}

// ID identifies this processor in configured processor-order lists.
func (p *spotifyProcessor) ID() string { return "spotify" }

// Detect matches open.spotify.com track links.
func (p *spotifyProcessor) Detect(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	_, ok := spotifyPathID(u.Host, u.Path, "track")
	return ok
}

// ExtractID pulls the track id out of an open.spotify.com track link.
func (p *spotifyProcessor) ExtractID(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("spotify: parse %q: %w", rawURL, err)
	}
	id, ok := spotifyPathID(u.Host, u.Path, "track")
	if !ok {
		return "", fmt.Errorf("spotify: no track id in %q", rawURL)
	}
	return id, nil
}

// spotifyPathID extracts kind's entity id from a path like "/track/<id>", tolerating a leading intl-locale segment.
func spotifyPathID(host, path, kind string) (string, bool) {
	host = strings.TrimPrefix(host, "www.")
	if host != "open.spotify.com" {
		return "", false
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i := 0; i < len(segments)-1; i++ {
		if segments[i] == kind && spotifyIDPattern.MatchString(segments[i+1]) {
			return segments[i+1], true
		}
	}
	return "", false
}

// spotifyIDFromURI pulls the id out of a "spotify:<kind>:<id>" URI.
func spotifyIDFromURI(uri string) string {
	parts := strings.Split(uri, ":")
	if len(parts) != 3 {
		return ""
	}
	return parts[2]
}

// anonymousToken returns a cached anonymous pathfinder bearer token, refreshing it from an embed page when stale.
func (p *spotifyProcessor) anonymousToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	valid := p.token != "" && time.Until(p.tokenExpiresAt) > spotifyTokenSkew
	token := p.token
	p.mu.Unlock()
	if valid {
		return token, nil
	}
	return p.refreshToken(ctx)
}

// refreshToken bootstraps a fresh anonymous token off the bootstrap track's embed page and caches it.
func (p *spotifyProcessor) refreshToken(ctx context.Context) (string, error) {
	reqURL := "https://open.spotify.com/embed/track/" + spotifyTokenBootstrapID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("spotify: bootstrap anonymous token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("spotify: bootstrap anonymous token: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("spotify: bootstrap anonymous token: read: %w", err)
	}

	m := spotifyNextDataPattern.FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("spotify: bootstrap anonymous token: no __NEXT_DATA__ in embed page")
	}
	var next map[string]any
	if err := json.Unmarshal(m[1], &next); err != nil {
		return "", fmt.Errorf("spotify: bootstrap anonymous token: decode __NEXT_DATA__: %w", err)
	}

	session, ok := navMap(next, "props", "pageProps", "state", "settings", "session")
	if !ok {
		return "", fmt.Errorf("spotify: bootstrap anonymous token: embed session missing")
	}
	token, _ := navString(session, "accessToken")
	if token == "" {
		return "", fmt.Errorf("spotify: bootstrap anonymous token: no accessToken in embed session")
	}
	expiresMs, _ := session["accessTokenExpirationTimestampMs"].(float64)

	p.mu.Lock()
	p.token = token
	p.tokenExpiresAt = time.UnixMilli(int64(expiresMs))
	p.mu.Unlock()
	return token, nil
}

// pathfinderQuery runs op with variables and returns the response's "data" object, retrying once on a stale token.
func (p *spotifyProcessor) pathfinderQuery(ctx context.Context, op spotifyOperation, variables map[string]any) (map[string]any, error) {
	token, err := p.anonymousToken(ctx)
	if err != nil {
		return nil, err
	}
	data, status, err := p.doPathfinderQuery(ctx, op, variables, token)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized {
		token, err = p.refreshToken(ctx)
		if err != nil {
			return nil, err
		}
		data, status, err = p.doPathfinderQuery(ctx, op, variables, token)
		if err != nil {
			return nil, err
		}
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("spotify: %s: status %d", op.name, status)
	}
	return data, nil
}

// doPathfinderQuery performs one pathfinder request for op, returning the response's data object and raw HTTP status.
func (p *spotifyProcessor) doPathfinderQuery(ctx context.Context, op spotifyOperation, variables map[string]any, token string) (map[string]any, int, error) {
	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return nil, 0, err
	}
	extensionsJSON, err := json.Marshal(map[string]any{
		"persistedQuery": map[string]any{"version": 1, "sha256Hash": op.hash},
	})
	if err != nil {
		return nil, 0, err
	}

	q := url.Values{}
	q.Set("operationName", op.name)
	q.Set("variables", string(variablesJSON))
	q.Set("extensions", string(extensionsJSON))
	reqURL := spotifyPathfinderURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("app-platform", "WebPlayer")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("spotify: %s: %w", op.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, 0, fmt.Errorf("spotify: %s: decode: %w", op.name, err)
	}
	data, ok := navMap(body, "data")
	if !ok {
		return nil, 0, fmt.Errorf("spotify: %s: unexpected response shape", op.name)
	}
	return data, resp.StatusCode, nil
}

// FetchMetadata fetches a track's title, duration, artists, and album via pathfinder's getTrack query.
func (p *spotifyProcessor) FetchMetadata(ctx context.Context, id string) (Metadata, bool, error) {
	data, err := p.pathfinderQuery(ctx, spotifyGetTrackOp, map[string]any{"uri": "spotify:track:" + id})
	if err != nil {
		return Metadata{}, false, fmt.Errorf("spotify: fetch track %s: %w", id, err)
	}
	track, ok := navMap(data, "trackUnion")
	if !ok {
		return Metadata{}, false, fmt.Errorf("spotify: track %s not found", id)
	}
	return spotifyParseTrack(track, id)
}

// spotifyParseTrack builds Metadata from a pathfinder trackUnion (as returned by getTrack).
func spotifyParseTrack(track map[string]any, fallbackID string) (Metadata, bool, error) {
	name, _ := navString(track, "name")
	if name == "" {
		return Metadata{}, false, fmt.Errorf("spotify: track %s missing name", fallbackID)
	}
	trackID, _ := navString(track, "id")
	if trackID == "" {
		trackID = fallbackID
	}

	meta := Metadata{SongName: name, ExtractedID: trackID, Artists: spotifyTrackArtists(track)}
	if d, ok := navMap(track, "duration"); ok {
		if ms, ok := d["totalMilliseconds"].(float64); ok {
			meta.DurationMs = int32(ms)
		}
	}

	if album, ok := navMap(track, "albumOfTrack"); ok {
		albumName, _ := navString(album, "name")
		albumURI, _ := navString(album, "uri")
		albumID := spotifyIDFromURI(albumURI)
		if albumName != "" && albumID != "" {
			var thumbnailURL string
			if sources, ok := navSlice(album, "coverArt", "sources"); ok {
				thumbnailURL = spotifyBestSourceThumbnail(sources)
			}
			meta.Album = &AlbumMetadata{Name: albumName, ExtractedID: albumID, ThumbnailURL: thumbnailURL}
			meta.ThumbnailURL = thumbnailURL
		}
	}
	if meta.ThumbnailURL == "" {
		if sources, ok := navSlice(track, "visualIdentity", "squareCoverImage", "sources"); ok {
			meta.ThumbnailURL = spotifyBestSourceThumbnail(sources)
		}
	}
	return meta, true, nil
}

// spotifyTrackArtists merges a pathfinder track's firstArtist+otherArtists (Spotify splits credits across the two).
func spotifyTrackArtists(track map[string]any) []ArtistMetadata {
	var artists []ArtistMetadata
	if items, ok := navSlice(track, "firstArtist", "items"); ok {
		artists = append(artists, spotifyArtistRefs(items)...)
	}
	if items, ok := navSlice(track, "otherArtists", "items"); ok {
		artists = append(artists, spotifyArtistRefs(items)...)
	}
	return artists
}

// spotifyArtistRefs converts a pathfinder artist-ref items array into ArtistMetadata, deriving id from uri if absent.
func spotifyArtistRefs(items []any) []ArtistMetadata {
	artists := make([]ArtistMetadata, 0, len(items))
	for _, raw := range items {
		item, ok := asMap(raw)
		if !ok {
			continue
		}
		name, _ := navString(item, "profile", "name")
		if name == "" {
			continue
		}
		id, _ := navString(item, "id")
		if id == "" {
			uri, _ := navString(item, "uri")
			id = spotifyIDFromURI(uri)
		}
		artists = append(artists, ArtistMetadata{Name: name, ExtractedID: id})
	}
	return artists
}

// FetchMetadataByQuery searches Spotify for q's song/artist text and returns the top track hit's full metadata.
func (p *spotifyProcessor) FetchMetadataByQuery(ctx context.Context, q Query) (Metadata, bool, error) {
	terms := strings.TrimSpace(q.Song + " " + q.Artist)
	if terms == "" {
		return Metadata{}, false, fmt.Errorf("spotify: empty query")
	}

	data, err := p.pathfinderQuery(ctx, spotifySearchOp, map[string]any{
		"searchTerm": terms, "offset": 0, "limit": 10, "numberOfTopResults": 5,
		"includeAudiobooks": true, "includePreReleases": true,
		"includeAlbumPreReleases": false, "includeAuthors": false, "includeEpisodeContentRatingsV2": false,
	})
	if err != nil {
		return Metadata{}, false, fmt.Errorf("spotify: search: %w", err)
	}
	items, ok := navSlice(data, "searchV2", "tracksV2", "items")
	if !ok || len(items) == 0 {
		return Metadata{}, false, nil
	}
	track, ok := navMap(items[0], "item", "data")
	if !ok {
		return Metadata{}, false, nil
	}

	name, _ := navString(track, "name")
	id, _ := navString(track, "id")
	if name == "" || id == "" {
		return Metadata{}, false, nil
	}

	meta := Metadata{SongName: name, ExtractedID: id}
	if d, ok := track["duration"].(map[string]any); ok {
		if ms, ok := d["totalMilliseconds"].(float64); ok {
			meta.DurationMs = int32(ms)
		}
	}
	if artistItems, ok := navSlice(track, "artists", "items"); ok {
		meta.Artists = spotifyArtistRefs(artistItems)
	}
	if album, ok := navMap(track, "albumOfTrack"); ok {
		albumName, _ := navString(album, "name")
		albumID, _ := navString(album, "id")
		if albumName != "" && albumID != "" {
			var thumbnailURL string
			if sources, ok := navSlice(album, "coverArt", "sources"); ok {
				thumbnailURL = spotifyBestSourceThumbnail(sources)
			}
			meta.Album = &AlbumMetadata{Name: albumName, ExtractedID: albumID, ThumbnailURL: thumbnailURL}
			meta.ThumbnailURL = thumbnailURL
		}
	}
	return meta, true, nil
}

// FetchAlbum fetches an album's metadata and full track listing, including each track's per-artist name and id.
func (p *spotifyProcessor) FetchAlbum(ctx context.Context, id string) (AlbumMetadata, error) {
	data, err := p.pathfinderQuery(ctx, spotifyGetAlbumOp, map[string]any{
		"uri": "spotify:album:" + id, "locale": "", "offset": 0, "limit": spotifyAlbumTrackLimit,
	})
	if err != nil {
		return AlbumMetadata{}, fmt.Errorf("spotify: fetch album %s: %w", id, err)
	}
	album, ok := navMap(data, "albumUnion")
	if !ok {
		return AlbumMetadata{}, fmt.Errorf("spotify: album %s not found", id)
	}

	name, _ := navString(album, "name")
	if name == "" {
		return AlbumMetadata{}, fmt.Errorf("spotify: album %s missing name", id)
	}

	var thumbnailURL string
	if sources, ok := navSlice(album, "coverArt", "sources"); ok {
		thumbnailURL = spotifyBestSourceThumbnail(sources)
	}

	items, _ := navSlice(album, "tracksV2", "items")
	tracks := make([]AlbumTrack, 0, len(items))
	for i, raw := range items {
		item, ok := asMap(raw)
		if !ok {
			continue
		}
		track, ok := navMap(item, "track")
		if !ok {
			continue
		}
		trackName, _ := navString(track, "name")
		trackURI, _ := navString(track, "uri")
		trackID := spotifyIDFromURI(trackURI)
		if trackName == "" || trackID == "" {
			continue
		}

		var durationMs int32
		if d, ok := navMap(track, "duration"); ok {
			if ms, ok := d["totalMilliseconds"].(float64); ok {
				durationMs = int32(ms)
			}
		}

		var artists []ArtistMetadata
		if artistItems, ok := navSlice(track, "artists", "items"); ok {
			artists = spotifyArtistRefs(artistItems)
		}

		tracks = append(tracks, AlbumTrack{
			Name: trackName, ExtractedID: trackID, ThumbnailURL: thumbnailURL,
			DurationMs: durationMs, TrackNumber: int32(i + 1), Artists: artists,
		})
	}

	return AlbumMetadata{Name: name, ExtractedID: id, ThumbnailURL: thumbnailURL, Songs: tracks}, nil
}

// FetchArtist fetches an artist's profile via pathfinder's queryArtistOverview query, including their bio.
func (p *spotifyProcessor) FetchArtist(ctx context.Context, id string) (ArtistMetadata, error) {
	data, err := p.pathfinderQuery(ctx, spotifyArtistOverview, map[string]any{
		"uri": "spotify:artist:" + id, "locale": "", "includePrerelease": false,
	})
	if err != nil {
		return ArtistMetadata{}, fmt.Errorf("spotify: fetch artist %s: %w", id, err)
	}
	artist, ok := navMap(data, "artistUnion")
	if !ok {
		return ArtistMetadata{}, fmt.Errorf("spotify: artist %s not found", id)
	}

	name, _ := navString(artist, "profile", "name")
	if name == "" {
		return ArtistMetadata{}, fmt.Errorf("spotify: artist %s missing name", id)
	}

	artistID, _ := navString(artist, "id")
	if artistID == "" {
		artistID = id
	}

	result := ArtistMetadata{Name: name, ExtractedID: artistID}
	if sources, ok := navSlice(artist, "visuals", "avatarImage", "sources"); ok {
		result.ThumbnailURL = spotifyBestSourceThumbnail(sources)
	}
	if bio, ok := navString(artist, "profile", "biography", "text"); ok {
		result.Description = bio
	}
	return result, nil
}

// State reports Spotify as always available -- pathfinder search and entity lookups need no credentials.
func (p *spotifyProcessor) State(context.Context) State {
	return State{CanDetect: true, CanLookup: true, CanFetchAlbum: true, CanFetchArtist: true}
}

// Type identifies this processor's source_type.
func (p *spotifyProcessor) Type() db.SourceType { return db.SourceTypeSpotify }

// spotifyBestSourceThumbnail picks the widest thumbnail URL from a pathfinder image "sources" array.
func spotifyBestSourceThumbnail(sources []any) string {
	var bestURL string
	var bestWidth float64
	for _, raw := range sources {
		item, ok := asMap(raw)
		if !ok {
			continue
		}
		imgURL, _ := item["url"].(string)
		width, _ := item["width"].(float64)
		if imgURL == "" {
			continue
		}
		if bestURL == "" || width > bestWidth {
			bestURL, bestWidth = imgURL, width
		}
	}
	return bestURL
}
