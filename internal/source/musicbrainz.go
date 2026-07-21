package source

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// mbidPattern matches a MusicBrainz recording page URL, capturing the MBID.
var mbidPattern = regexp.MustCompile(`musicbrainz\.org/recording/([0-9a-fA-F-]{36})`)

// musicBrainzProcessor resolves direct links and supports text search.
type musicBrainzProcessor struct {
	baseURL    string
	httpClient *http.Client
	limiter    *rate.Limiter
}

// NewMusicBrainzProcessor builds the processor against baseURL, rate-limited to requestsPerSecond (<= 0 means unlimited).
func NewMusicBrainzProcessor(baseURL string, requestsPerSecond int) Processor {
	limit := rate.Inf
	if requestsPerSecond > 0 {
		limit = rate.Limit(requestsPerSecond)
	}
	return &musicBrainzProcessor{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		limiter:    rate.NewLimiter(limit, 1),
	}
}

// ID identifies this processor in configured processor-order lists.
func (p *musicBrainzProcessor) ID() string { return "musicbrainz" }

// Detect matches MusicBrainz page links.
func (p *musicBrainzProcessor) Detect(url string) bool {
	return mbidPattern.MatchString(url)
}

// ExtractID returns the MBID captured from url.
func (p *musicBrainzProcessor) ExtractID(url string) (string, error) {
	m := mbidPattern.FindStringSubmatch(url)
	if m == nil {
		return "", fmt.Errorf("musicbrainz: no recording mbid in %q", url)
	}
	return m[1], nil
}

// mbzArtistCredit is one artist credit as MusicBrainz reports it.
type mbzArtistCredit struct {
	Artist struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"artist"`
}

// mbzRecording is the subset of the MusicBrainz recording lookup response Canto needs.
type mbzRecording struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Length       int32             `json:"length"`
	ArtistCredit []mbzArtistCredit `json:"artist-credit"`
	Releases     []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"releases"`
}

// mbzSearchResult is the subset of the MusicBrainz recording search response Canto needs.
type mbzSearchResult struct {
	Recordings []struct {
		mbzRecording
		Score int `json:"score"`
	} `json:"recordings"`
}

// mbzRelease is the subset of the MusicBrainz release lookup response Canto needs.
type mbzRelease struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	ArtistCredit []mbzArtistCredit `json:"artist-credit"`
	Media        []struct {
		Tracks []struct {
			Position  int    `json:"position"`
			Title     string `json:"title"`
			Length    int32  `json:"length"`
			Recording struct {
				ID           string            `json:"id"`
				Length       int32             `json:"length"`
				ArtistCredit []mbzArtistCredit `json:"artist-credit"`
			} `json:"recording"`
		} `json:"tracks"`
	} `json:"media"`
}

// FetchMetadata looks up the recording by MBID, pulling artist credits, first release, and cover art.
func (p *musicBrainzProcessor) FetchMetadata(ctx context.Context, id string) (Metadata, bool, error) {
	reqURL := fmt.Sprintf("%s/recording/%s?fmt=json&inc=artist-credits+releases", p.baseURL, id)
	var rec mbzRecording
	if err := p.get(ctx, reqURL, &rec); err != nil {
		return Metadata{}, false, err
	}
	return recordingMetadata(rec.Title, id, rec.Length, rec.ArtistCredit, rec.Releases), true, nil
}

// FetchMetadataByQuery searches MusicBrainz's recording index by artist/song text, returning the top-scoring result.
func (p *musicBrainzProcessor) FetchMetadataByQuery(ctx context.Context, q Query) (Metadata, bool, error) {
	query := fmt.Sprintf("recording:%q", q.Song)
	if q.Artist != "" {
		query += fmt.Sprintf(" AND artist:%q", q.Artist)
	}
	reqURL := fmt.Sprintf("%s/recording?query=%s&fmt=json&limit=1", p.baseURL, url.QueryEscape(query))

	var res mbzSearchResult
	if err := p.get(ctx, reqURL, &res); err != nil {
		return Metadata{}, false, err
	}
	if len(res.Recordings) == 0 {
		return Metadata{}, false, nil
	}

	top := res.Recordings[0]
	meta := recordingMetadata(top.Title, top.ID, top.Length, top.ArtistCredit, top.Releases)
	return meta, top.Score >= 90, nil
}

// recordingMetadata builds a Metadata from a recording's fields, shared by FetchMetadata and FetchMetadataByQuery.
func recordingMetadata(title, id string, length int32, credits []mbzArtistCredit, releases []struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}) Metadata {
	meta := Metadata{SongName: title, ExtractedID: id, DurationMs: length}
	for _, credit := range credits {
		meta.Artists = append(meta.Artists, ArtistMetadata{Name: credit.Artist.Name, ExtractedID: credit.Artist.ID})
	}
	if len(releases) > 0 {
		meta.Album = &AlbumMetadata{
			Name: releases[0].Title, ExtractedID: releases[0].ID,
			ThumbnailURL: fmt.Sprintf("https://coverartarchive.org/release/%s/front", releases[0].ID),
		}
	}
	return meta
}

// FetchAlbum looks up the release by MBID, pulling its full track listing.
func (p *musicBrainzProcessor) FetchAlbum(ctx context.Context, id string) (AlbumMetadata, error) {
	reqURL := fmt.Sprintf("%s/release/%s?fmt=json&inc=recordings+artist-credits", p.baseURL, id)
	var rel mbzRelease
	if err := p.get(ctx, reqURL, &rel); err != nil {
		return AlbumMetadata{}, err
	}

	var releaseArtists []ArtistMetadata
	for _, credit := range rel.ArtistCredit {
		releaseArtists = append(releaseArtists, ArtistMetadata{Name: credit.Artist.Name, ExtractedID: credit.Artist.ID})
	}

	album := AlbumMetadata{
		Name: rel.Title, ExtractedID: rel.ID,
		ThumbnailURL: fmt.Sprintf("https://coverartarchive.org/release/%s/front", rel.ID),
	}
	var trackNum int32
	for _, medium := range rel.Media {
		for _, track := range medium.Tracks {
			trackNum++
			num := int32(track.Position)
			if num == 0 {
				num = trackNum
			}
			length := track.Length
			if length == 0 {
				length = track.Recording.Length
			}

			var artists []ArtistMetadata
			for _, credit := range track.Recording.ArtistCredit {
				artists = append(artists, ArtistMetadata{Name: credit.Artist.Name, ExtractedID: credit.Artist.ID})
			}
			if len(artists) == 0 {
				artists = releaseArtists
			}

			album.Songs = append(album.Songs, AlbumTrack{
				Name: track.Title, ExtractedID: track.Recording.ID,
				DurationMs: length, TrackNumber: num, Artists: artists,
			})
		}
	}
	return album, nil
}

// mbzArtist is the subset of the MusicBrainz artist lookup response Canto needs.
type mbzArtist struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Disambiguation string `json:"disambiguation"`
}

// FetchArtist looks up the artist by MBID, pulling its name and disambiguation comment.
func (p *musicBrainzProcessor) FetchArtist(ctx context.Context, id string) (ArtistMetadata, error) {
	reqURL := fmt.Sprintf("%s/artist/%s?fmt=json", p.baseURL, id)
	var artist mbzArtist
	if err := p.get(ctx, reqURL, &artist); err != nil {
		return ArtistMetadata{}, err
	}
	if artist.Name == "" {
		return ArtistMetadata{}, fmt.Errorf("musicbrainz: artist %s not found", id)
	}
	return ArtistMetadata{Name: artist.Name, ExtractedID: id, Description: artist.Disambiguation}, nil
}

// State reports MusicBrainz as always available.
func (p *musicBrainzProcessor) State(context.Context) State {
	return State{CanDetect: true, CanLookup: true, CanFetchAlbum: true, CanFetchArtist: true}
}

// Type identifies this processor's source_type.
func (p *musicBrainzProcessor) Type() SourceType { return SourceTypeMusicbrainz }

// get performs a rate-limited GET request against reqURL and decodes the JSON response into out.
func (p *musicBrainzProcessor) get(ctx context.Context, reqURL string, out any) error {
	if err := p.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("musicbrainz: rate limit wait: %w", err)
	}
	slog.Debug("musicbrainz: request", "url", reqURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Canto/0.1 (+https://github.com/canto)")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("musicbrainz: fetch %s: %w", reqURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("musicbrainz: fetch %s: status %d", reqURL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("musicbrainz: decode %s: %w", reqURL, err)
	}
	return nil
}
