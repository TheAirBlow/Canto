package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"Canto/internal/db"
)

// lastFMProcessor looks up track metadata from Last.fm by artist/song text.
type lastFMProcessor struct {
	apiKey     string
	httpClient *http.Client
}

// NewLastFMProcessor builds the processor.
func NewLastFMProcessor(apiKey string) Processor {
	return &lastFMProcessor{apiKey: apiKey, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

// ID identifies this processor in configured processor-order lists.
func (p *lastFMProcessor) ID() string { return "lastfm" }

// Detect is a no-op.
func (p *lastFMProcessor) Detect(string) bool { return false }

// ExtractID is not supported and always returns an error.
func (p *lastFMProcessor) ExtractID(string) (string, error) {
	return "", fmt.Errorf("lastfm: does not support origin_url resolution")
}

// FetchMetadata is not supported and always returns an error.
func (p *lastFMProcessor) FetchMetadata(context.Context, string) (Metadata, bool, error) {
	return Metadata{}, false, fmt.Errorf("lastfm: does not support id-based lookup")
}

// FetchAlbum is unsupported; State().CanFetchAlbum is always false.
func (p *lastFMProcessor) FetchAlbum(context.Context, string) (AlbumMetadata, error) {
	return AlbumMetadata{}, fmt.Errorf("lastfm: does not support album lookup")
}

// lastFMArtistInfo is the subset of artist.getInfo Canto needs.
type lastFMArtistInfo struct {
	Artist struct {
		Name  string `json:"name"`
		Image []struct {
			Text string `json:"#text"`
			Size string `json:"size"`
		} `json:"image"`
		Bio struct {
			Summary string `json:"summary"`
		} `json:"bio"`
	} `json:"artist"`
}

// FetchArtist looks up name via artist.getInfo, pulling its bio summary and largest image.
func (p *lastFMProcessor) FetchArtist(ctx context.Context, name string) (ArtistMetadata, error) {
	reqURL := fmt.Sprintf(
		"https://ws.audioscrobbler.com/2.0/?method=artist.getinfo&api_key=%s&artist=%s&format=json",
		url.QueryEscape(p.apiKey), url.QueryEscape(name),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return ArtistMetadata{}, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ArtistMetadata{}, fmt.Errorf("lastfm: fetch artist %q: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ArtistMetadata{}, fmt.Errorf("lastfm: fetch artist %q: status %d", name, resp.StatusCode)
	}

	var info lastFMArtistInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return ArtistMetadata{}, fmt.Errorf("lastfm: decode artist %q: %w", name, err)
	}
	if info.Artist.Name == "" {
		return ArtistMetadata{}, fmt.Errorf("lastfm: artist %q not found", name)
	}

	var thumbnailURL string
	for _, img := range info.Artist.Image {
		if img.Text != "" {
			thumbnailURL = img.Text // Last.fm's image array is smallest-to-largest; keep the last non-empty one.
		}
	}
	return ArtistMetadata{Name: info.Artist.Name, ExtractedID: name, ThumbnailURL: thumbnailURL, Description: info.Artist.Bio.Summary}, nil
}

// lastFMTrackInfo is the subset of track.getInfo Canto needs.
type lastFMTrackInfo struct {
	Track struct {
		Name     string `json:"name"`
		Duration string `json:"duration"`
		Artist   struct {
			Name string `json:"name"`
		} `json:"artist"`
		Album struct {
			Title string `json:"title"`
		} `json:"album"`
	} `json:"track"`
}

// FetchMetadataByQuery looks up q's track via Last.fm's track.getInfo.
func (p *lastFMProcessor) FetchMetadataByQuery(ctx context.Context, q Query) (Metadata, bool, error) {
	reqURL := fmt.Sprintf(
		"https://ws.audioscrobbler.com/2.0/?method=track.getInfo&api_key=%s&artist=%s&track=%s&format=json",
		url.QueryEscape(p.apiKey), url.QueryEscape(q.Artist), url.QueryEscape(q.Song),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return Metadata{}, false, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return Metadata{}, false, fmt.Errorf("lastfm: fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Metadata{}, false, nil
	}

	var info lastFMTrackInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return Metadata{}, false, fmt.Errorf("lastfm: decode: %w", err)
	}
	if info.Track.Name == "" {
		return Metadata{}, false, nil
	}

	meta := Metadata{SongName: info.Track.Name}
	if info.Track.Album.Title != "" {
		meta.Album = &AlbumMetadata{Name: info.Track.Album.Title}
	}
	if info.Track.Artist.Name != "" {
		meta.Artists = []ArtistMetadata{{Name: info.Track.Artist.Name}}
	}
	if ms, err := strconv.Atoi(info.Track.Duration); err == nil {
		meta.DurationMs = int32(ms)
	}
	return meta, true, nil
}

// State reports whether Last.fm lookups are available.
func (p *lastFMProcessor) State(context.Context) State {
	return State{CanDetect: false, CanLookup: p.apiKey != "", CanFetchAlbum: false, CanFetchArtist: p.apiKey != ""}
}

// Type identifies this processor's source_type.
func (p *lastFMProcessor) Type() db.SourceType { return db.SourceTypeLastfm }
