package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"Canto/internal/db"
)

// deezerIDPattern matches a Deezer numeric entity id.
var deezerIDPattern = regexp.MustCompile(`^\d+$`)

// deezerMaxTrackPages bounds how many paginated /tracks responses FetchAlbum follows.
const deezerMaxTrackPages = 20

// deezerProcessor resolves deezer.com track links and looks up track/album/artist metadata via Deezer's public API.
type deezerProcessor struct {
	httpClient *http.Client
}

// NewDeezerProcessor builds the processor.
func NewDeezerProcessor() Processor {
	return &deezerProcessor{httpClient: &http.Client{Timeout: 10 * time.Second}}
}

// ID identifies this processor in configured processor-order lists.
func (p *deezerProcessor) ID() string { return "deezer" }

// Detect matches deezer.com track links, with or without a leading language segment (e.g. /en/track/123).
func (p *deezerProcessor) Detect(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	_, ok := deezerPathID(u.Host, u.Path, "track")
	return ok
}

// ExtractID pulls the track id out of a deezer.com track link.
func (p *deezerProcessor) ExtractID(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("deezer: parse %q: %w", rawURL, err)
	}
	id, ok := deezerPathID(u.Host, u.Path, "track")
	if !ok {
		return "", fmt.Errorf("deezer: no track id in %q", rawURL)
	}
	return id, nil
}

// deezerPathID extracts a numeric entity id from a path shaped like "/track/<id>" or "/en/track/<id>".
func deezerPathID(host, path, kind string) (string, bool) {
	host = strings.TrimPrefix(host, "www.")
	if host != "deezer.com" {
		return "", false
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i := 0; i < len(segments)-1; i++ {
		if segments[i] == kind && deezerIDPattern.MatchString(segments[i+1]) {
			return segments[i+1], true
		}
	}
	return "", false
}

// deezerError is the error envelope Deezer returns (with HTTP 200) for an invalid id or bad request.
type deezerError struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// deezerArtistRef is an artist as embedded in a track/album response.
type deezerArtistRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// deezerTrack is the subset of GET /track/{id} Canto needs.
type deezerTrack struct {
	deezerError
	ID       int64           `json:"id"`
	Title    string          `json:"title"`
	Duration int32           `json:"duration"`
	Artist   deezerArtistRef `json:"artist"`
	Album    deezerAlbumRef  `json:"album"`
}

// deezerAlbumRef is an album as embedded in a track response.
type deezerAlbumRef struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	CoverBig string `json:"cover_big"`
}

// FetchMetadata fetches a track's title, duration, artist, and album via GET /track/{id}.
func (p *deezerProcessor) FetchMetadata(ctx context.Context, id string) (Metadata, bool, error) {
	var t deezerTrack
	if err := p.get(ctx, "https://api.deezer.com/track/"+id, &t); err != nil {
		return Metadata{}, false, err
	}
	if t.Error != nil {
		return Metadata{}, false, fmt.Errorf("deezer: track %s: %s", id, t.Error.Message)
	}

	meta := Metadata{SongName: t.Title, ExtractedID: strconv.FormatInt(t.ID, 10), DurationMs: t.Duration * 1000}
	if t.Artist.Name != "" {
		meta.Artists = []ArtistMetadata{{Name: t.Artist.Name, ExtractedID: strconv.FormatInt(t.Artist.ID, 10)}}
	}
	if t.Album.Title != "" {
		meta.Album = &AlbumMetadata{
			Name: t.Album.Title, ExtractedID: strconv.FormatInt(t.Album.ID, 10), ThumbnailURL: t.Album.CoverBig,
		}
		meta.ThumbnailURL = t.Album.CoverBig
	}
	return meta, true, nil
}

// deezerSearchResult is the subset of Deezer's search response Canto needs.
type deezerSearchResult struct {
	Data []deezerTrack `json:"data"`
}

// FetchMetadataByQuery searches Deezer for q's artist/song text, returning the top hit.
func (p *deezerProcessor) FetchMetadataByQuery(ctx context.Context, q Query) (Metadata, bool, error) {
	query := fmt.Sprintf("track:\"%s\" artist:\"%s\"", q.Song, q.Artist)
	reqURL := "https://api.deezer.com/search?q=" + url.QueryEscape(query)

	var res deezerSearchResult
	if err := p.get(ctx, reqURL, &res); err != nil {
		return Metadata{}, false, err
	}
	if len(res.Data) == 0 {
		return Metadata{}, false, nil
	}

	top := res.Data[0]
	meta := Metadata{SongName: top.Title, ExtractedID: strconv.FormatInt(top.ID, 10), DurationMs: top.Duration * 1000}
	if top.Album.Title != "" {
		meta.Album = &AlbumMetadata{
			Name: top.Album.Title, ExtractedID: strconv.FormatInt(top.Album.ID, 10), ThumbnailURL: top.Album.CoverBig,
		}
		meta.ThumbnailURL = top.Album.CoverBig
	}
	if top.Artist.Name != "" {
		meta.Artists = []ArtistMetadata{{Name: top.Artist.Name, ExtractedID: strconv.FormatInt(top.Artist.ID, 10)}}
	}
	return meta, true, nil
}

// deezerAlbum is the subset of GET /album/{id} Canto needs.
type deezerAlbum struct {
	deezerError
	ID       int64           `json:"id"`
	Title    string          `json:"title"`
	CoverBig string          `json:"cover_big"`
	Artist   deezerArtistRef `json:"artist"`
}

// deezerAlbumTracksPage is one page of GET /album/{id}/tracks.
type deezerAlbumTracksPage struct {
	deezerError
	Data []struct {
		ID            int64           `json:"id"`
		Title         string          `json:"title"`
		Duration      int32           `json:"duration"`
		TrackPosition int32           `json:"track_position"`
		Artist        deezerArtistRef `json:"artist"`
	} `json:"data"`
	Next string `json:"next"`
}

// FetchAlbum fetches an album's metadata and full track listing via GET /album/{id} and its paginated /tracks endpoint.
func (p *deezerProcessor) FetchAlbum(ctx context.Context, id string) (AlbumMetadata, error) {
	var a deezerAlbum
	if err := p.get(ctx, "https://api.deezer.com/album/"+id, &a); err != nil {
		return AlbumMetadata{}, err
	}
	if a.Error != nil {
		return AlbumMetadata{}, fmt.Errorf("deezer: album %s: %s", id, a.Error.Message)
	}

	album := AlbumMetadata{Name: a.Title, ExtractedID: strconv.FormatInt(a.ID, 10), ThumbnailURL: a.CoverBig}

	var albumArtists []ArtistMetadata
	if a.Artist.Name != "" {
		albumArtists = []ArtistMetadata{{Name: a.Artist.Name, ExtractedID: strconv.FormatInt(a.Artist.ID, 10)}}
	}

	tracksURL := fmt.Sprintf("https://api.deezer.com/album/%s/tracks", id)
	for page := 0; tracksURL != "" && page < deezerMaxTrackPages; page++ {
		var res deezerAlbumTracksPage
		if err := p.get(ctx, tracksURL, &res); err != nil {
			return AlbumMetadata{}, err
		}
		if res.Error != nil {
			return AlbumMetadata{}, fmt.Errorf("deezer: album %s tracks: %s", id, res.Error.Message)
		}
		for _, t := range res.Data {
			artists := albumArtists
			if t.Artist.Name != "" {
				artists = []ArtistMetadata{{Name: t.Artist.Name, ExtractedID: strconv.FormatInt(t.Artist.ID, 10)}}
			}
			album.Songs = append(album.Songs, AlbumTrack{
				Name: t.Title, ExtractedID: strconv.FormatInt(t.ID, 10), ThumbnailURL: a.CoverBig,
				DurationMs: t.Duration * 1000, TrackNumber: t.TrackPosition, Artists: artists,
			})
		}
		tracksURL = res.Next
	}

	return album, nil
}

// deezerArtist is the subset of GET /artist/{id} Canto needs.
type deezerArtist struct {
	deezerError
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	PictureBig string `json:"picture_big"`
}

// FetchArtist fetches an artist's profile metadata (name, thumbnail) via GET /artist/{id}.
func (p *deezerProcessor) FetchArtist(ctx context.Context, id string) (ArtistMetadata, error) {
	var a deezerArtist
	if err := p.get(ctx, "https://api.deezer.com/artist/"+id, &a); err != nil {
		return ArtistMetadata{}, err
	}
	if a.Error != nil {
		return ArtistMetadata{}, fmt.Errorf("deezer: artist %s: %s", id, a.Error.Message)
	}
	return ArtistMetadata{Name: a.Name, ExtractedID: strconv.FormatInt(a.ID, 10), ThumbnailURL: a.PictureBig}, nil
}

// State reports Deezer as always available -- its public read API needs no credentials.
func (p *deezerProcessor) State(context.Context) State {
	return State{CanDetect: true, CanLookup: true, CanFetchAlbum: true, CanFetchArtist: true}
}

// Type identifies this processor's source_type.
func (p *deezerProcessor) Type() db.SourceType { return db.SourceTypeDeezer }

// get performs a GET request against reqURL and decodes the JSON response into out.
func (p *deezerProcessor) get(ctx context.Context, reqURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("deezer: fetch %s: %w", reqURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("deezer: fetch %s: status %d", reqURL, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("deezer: decode %s: %w", reqURL, err)
	}
	return nil
}
