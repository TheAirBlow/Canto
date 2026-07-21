package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ytmSongsFilterParam restricts a YTM search to the "Songs" category.
const ytmSongsFilterParam = "Eg-KAQwIARAAGAAgACgAMABqChAEEAMQCRAFEAo%3D"

// durationPattern matches a bare "m:ss" or "h:mm:ss" duration run, e.g. from a track panel byline.
var durationPattern = regexp.MustCompile(`^\d+(:\d+)+$`)

// lockoutBase is the initial lockout duration on a suspected rate limit.
const lockoutBase = 5 * time.Second

// lockoutMax caps how long consecutive suspected rate limits can extend the lockout.
const lockoutMax = 5 * time.Minute

// youtubeProcessor covers YouTube and YouTube Music through internal YTM endpoints.
type youtubeProcessor struct {
	httpClient *http.Client

	mu               sync.Mutex
	lockedUntil      time.Time
	consecutiveFails int
}

// NewYouTubeProcessor builds the processor.
func NewYouTubeProcessor() Processor {
	return &youtubeProcessor{httpClient: &http.Client{Timeout: 10 * time.Second}}
}

// ID identifies this processor in configured processor-order lists.
func (p *youtubeProcessor) ID() string { return "ytmusic" }

// Detect matches YouTube and YouTube Music links.
func (p *youtubeProcessor) Detect(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.TrimPrefix(u.Host, "www.")
	switch host {
	case "youtube.com", "music.youtube.com":
		return u.Path == "/watch" && u.Query().Get("v") != ""
	case "youtu.be":
		return strings.Trim(u.Path, "/") != ""
	}
	return false
}

// ExtractID pulls the video id from either the `v` query param or the youtu.be path.
func (p *youtubeProcessor) ExtractID(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("youtube: parse %q: %w", rawURL, err)
	}
	host := strings.TrimPrefix(u.Host, "www.")
	if host == "youtu.be" {
		return strings.Trim(u.Path, "/"), nil
	}
	if id := u.Query().Get("v"); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("youtube: no video id in %q", rawURL)
}

// checkLocked returns an error without making a network call if a suspected rate limit is still in effect.
func (p *youtubeProcessor) checkLocked() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if remaining := time.Until(p.lockedUntil); remaining > 0 {
		return fmt.Errorf("youtube: locked out for %s after suspected rate limiting", remaining.Round(time.Second))
	}
	return nil
}

// lockOut engages an exponentially growing lockout after a suspected rate limit, logging reason.
func (p *youtubeProcessor) lockOut(reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	dur := min(lockoutBase*(1<<p.consecutiveFails), lockoutMax)
	p.lockedUntil = time.Now().Add(dur)
	p.consecutiveFails++
	slog.Warn("youtube: suspected rate limit, locking out", "reason", reason, "duration", dur, "consecutive", p.consecutiveFails)
}

// recover resets the consecutive-failure count after a fully successful response.
func (p *youtubeProcessor) recover() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutiveFails = 0
}

// innertubeContext is the WEB_REMIX client context YTM's internal endpoints expect.
type innertubeContext struct {
	Client struct {
		ClientName    string `json:"clientName"`
		ClientVersion string `json:"clientVersion"`
		Hl            string `json:"hl"`
		Gl            string `json:"gl"`
	} `json:"client"`
	User struct{} `json:"user"`
}

// newInnertubeContext creates a new innertubeContext.
func newInnertubeContext() innertubeContext {
	var ctx innertubeContext
	ctx.Client.ClientName = "WEB_REMIX"
	ctx.Client.ClientVersion = "1.20260707.12.00"
	ctx.Client.Hl = "en"
	ctx.Client.Gl = "US"
	return ctx
}

// ytmCall POSTs body to a YTM youtubei/v1 endpoint and returns the decoded JSON tree.
func (p *youtubeProcessor) ytmCall(ctx context.Context, endpoint string, body any) (map[string]any, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	reqURL := fmt.Sprintf("https://music.youtube.com/youtubei/v1/%s?alt=json", endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:152.0) Gecko/20100101 Firefox/152.0")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://music.youtube.com")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.lockOut("transport error")
		return nil, fmt.Errorf("youtube: ytm %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		p.lockOut(fmt.Sprintf("http status %d", resp.StatusCode))
		return nil, fmt.Errorf("youtube: ytm %s: status %d", endpoint, resp.StatusCode)
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		p.lockOut("decode failure")
		return nil, fmt.Errorf("youtube: ytm %s decode: %w", endpoint, err)
	}
	return out, nil
}

// FetchMetadata fetches a video's title, duration, artists and album; confident reports whether it's catalog music rather than a plain video.
func (p *youtubeProcessor) FetchMetadata(ctx context.Context, id string) (Metadata, bool, error) {
	if err := p.checkLocked(); err != nil {
		return Metadata{}, false, err
	}

	nextResp, err := p.ytmCall(ctx, "next", map[string]any{
		"context":                       newInnertubeContext(),
		"enablePersistentPlaylistPanel": true,
		"isAudioOnly":                   true,
		"videoId":                       id,
	})
	if err != nil {
		return Metadata{}, false, err
	}

	contents, ok := navSlice(nextResp,
		"contents", "singleColumnMusicWatchNextResultsRenderer", "tabbedRenderer", "watchNextTabbedResultsRenderer",
		"tabs", 0, "tabRenderer", "content", "musicQueueRenderer", "content", "playlistPanelRenderer", "contents")
	if !ok {
		p.lockOut("unexpected next response shape")
		return Metadata{}, false, fmt.Errorf("youtube: unexpected next response shape for %s", id)
	}

	var trackData map[string]any
	for _, raw := range contents {
		entry, ok := asMap(raw)
		if !ok {
			continue
		}
		if wrapper, ok := navMap(entry, "playlistPanelVideoWrapperRenderer", "primaryRenderer"); ok {
			entry = wrapper
		}
		renderer, ok := navMap(entry, "playlistPanelVideoRenderer")
		if !ok {
			continue
		}
		if _, unplayable := renderer["unplayableText"]; unplayable {
			continue
		}
		trackData = renderer
		break
	}
	if trackData == nil {
		return Metadata{}, false, fmt.Errorf("youtube: no playable track found for %s", id)
	}

	title, ok := navString(trackData, "title", "runs", 0, "text")
	if !ok {
		p.lockOut("unexpected track title shape")
		return Metadata{}, false, fmt.Errorf("youtube: unexpected track shape for %s", id)
	}

	meta := Metadata{SongName: title, ExtractedID: id}
	if lengthText, ok := navString(trackData, "lengthText", "runs", 0, "text"); ok {
		meta.DurationMs = toSeconds(lengthText) * 1000
	}
	if thumbs, ok := navSlice(trackData, "thumbnail", "thumbnails"); ok {
		meta.ThumbnailURL = bestThumbnail(thumbs)
	}
	if byline, ok := trackData["longBylineText"]; ok {
		artists, album, _ := parseSongRuns(extractRuns(byline))
		meta.Artists, meta.Album = artists, album
	}

	musicVideoType, _ := navString(trackData,
		"navigationEndpoint", "watchEndpoint", "watchEndpointMusicSupportedConfigs", "watchEndpointMusicConfig", "musicVideoType")
	p.recover()
	return meta, musicVideoType != "", nil
}

// FetchMetadataByQuery searches YTM for q's song/artist text and fetches full metadata for the top hit.
func (p *youtubeProcessor) FetchMetadataByQuery(ctx context.Context, q Query) (Metadata, bool, error) {
	if err := p.checkLocked(); err != nil {
		return Metadata{}, false, err
	}

	terms := strings.TrimSpace(q.Song + " " + q.Artist)
	if terms == "" {
		return Metadata{}, false, fmt.Errorf("youtube: empty query")
	}

	searchResp, err := p.ytmCall(ctx, "search", map[string]any{
		"context": newInnertubeContext(), "query": terms, "params": ytmSongsFilterParam,
	})
	if err != nil {
		return Metadata{}, false, err
	}

	videoID, ok := firstSearchVideoID(searchResp)
	if !ok {
		return Metadata{}, false, nil
	}
	return p.FetchMetadata(ctx, videoID)
}

// FetchAlbum fetches an album's metadata and full track listing.
func (p *youtubeProcessor) FetchAlbum(ctx context.Context, id string) (AlbumMetadata, error) {
	if err := p.checkLocked(); err != nil {
		return AlbumMetadata{}, err
	}

	browseResp, err := p.ytmCall(ctx, "browse", map[string]any{"context": newInnertubeContext(), "browseId": id})
	if err != nil {
		return AlbumMetadata{}, err
	}

	header, ok := navMap(browseResp, "contents", "twoColumnBrowseResultsRenderer", "tabs", 0, "tabRenderer", "content",
		"sectionListRenderer", "contents", 0, "musicResponsiveHeaderRenderer")
	if !ok {
		p.lockOut("unexpected album browse response shape")
		return AlbumMetadata{}, fmt.Errorf("youtube: unexpected album browse response shape for %s", id)
	}
	shelf, ok := navMap(browseResp, "contents", "twoColumnBrowseResultsRenderer", "secondaryContents",
		"sectionListRenderer", "contents", 0, "musicShelfRenderer")
	if !ok {
		p.lockOut("unexpected album browse response shape")
		return AlbumMetadata{}, fmt.Errorf("youtube: unexpected album browse response shape for %s", id)
	}

	var thumbnailURL string
	if thumbs, ok := navSlice(header, "thumbnail", "musicThumbnailRenderer", "thumbnail", "thumbnails"); ok {
		thumbnailURL = bestThumbnail(thumbs)
	}

	var albumArtists []ArtistMetadata
	if strapline, ok := header["straplineTextOne"]; ok {
		for _, run := range extractRuns(strapline) {
			text, _ := run["text"].(string)
			if text == "" {
				continue
			}
			browseID, _ := runBrowseID(run)
			albumArtists = append(albumArtists, ArtistMetadata{Name: text, ExtractedID: browseID})
		}
	}
	albumName, _ := navString(header, "title", "runs", 0, "text")

	shelfContents, _ := navSlice(shelf, "contents")
	tracks := make([]AlbumTrack, 0, len(shelfContents))
	for i, entryRaw := range shelfContents {
		item, ok := navMap(entryRaw, "musicResponsiveListItemRenderer")
		if !ok {
			continue
		}
		playEndpoint, ok := navMap(item,
			"overlay", "musicItemThumbnailOverlayRenderer", "content", "musicPlayButtonRenderer", "playNavigationEndpoint", "watchEndpoint")
		if !ok {
			continue
		}
		playedVideoID, ok := navString(playEndpoint, "videoId")
		if !ok {
			continue
		}

		videoID := playedVideoID
		menuItems, _ := navSlice(item, "menu", "menuRenderer", "items")
		for _, menuItemRaw := range menuItems {
			creditsBrowseID, ok := navString(menuItemRaw, "menuNavigationItemRenderer", "navigationEndpoint", "browseEndpoint", "browseId")
			if ok && strings.HasPrefix(creditsBrowseID, "MPTC") {
				videoID = strings.TrimPrefix(creditsBrowseID, "MPTC")
				break
			}
		}

		title, _ := navString(item, "flexColumns", 0, "musicResponsiveListItemFlexColumnRenderer", "text", "runs", 0, "text")

		var artists []ArtistMetadata
		if text, ok := navMap(item, "flexColumns", 1, "musicResponsiveListItemFlexColumnRenderer", "text"); ok {
			artists, _, _ = parseSongRuns(extractRuns(text))
		}
		if len(artists) == 0 {
			artists = albumArtists
		}

		var durationMs int32
		if durationText, ok := navString(item, "fixedColumns", 0, "musicResponsiveListItemFixedColumnRenderer", "text", "runs", 0, "text"); ok {
			durationMs = toSeconds(durationText) * 1000
		}

		tracks = append(tracks, AlbumTrack{
			Name: title, ExtractedID: videoID, ThumbnailURL: thumbnailURL,
			DurationMs: durationMs, TrackNumber: int32(i + 1), Artists: artists,
		})
	}

	p.recover()
	return AlbumMetadata{Name: albumName, ExtractedID: id, ThumbnailURL: thumbnailURL, Songs: tracks}, nil
}

// FetchArtist fetches an artist's profile metadata (name, description, thumbnail).
func (p *youtubeProcessor) FetchArtist(ctx context.Context, id string) (ArtistMetadata, error) {
	if err := p.checkLocked(); err != nil {
		return ArtistMetadata{}, err
	}

	id = strings.TrimPrefix(id, "MPLA")

	browseResp, err := p.ytmCall(ctx, "browse", map[string]any{"context": newInnertubeContext(), "browseId": id})
	if err != nil {
		return ArtistMetadata{}, err
	}

	header, ok := navMap(browseResp, "header", "musicImmersiveHeaderRenderer")
	if !ok {
		p.lockOut("unexpected artist browse response shape")
		return ArtistMetadata{}, fmt.Errorf("youtube: unexpected artist browse response shape for %s", id)
	}
	results, ok := navSlice(browseResp,
		"contents", "singleColumnBrowseResultsRenderer", "tabs", 0, "tabRenderer", "content", "sectionListRenderer", "contents")
	if !ok {
		p.lockOut("unexpected artist browse response shape")
		return ArtistMetadata{}, fmt.Errorf("youtube: unexpected artist browse response shape for %s", id)
	}

	var description string
	for _, raw := range results {
		shelf, ok := navMap(raw, "musicDescriptionShelfRenderer")
		if !ok {
			continue
		}
		var sb strings.Builder
		for _, run := range extractRuns(shelf["description"]) {
			if text, ok := run["text"].(string); ok {
				sb.WriteString(text)
			}
		}
		description = sb.String()
		break
	}

	name, _ := navString(header, "title", "runs", 0, "text")
	var thumbnailURL string
	if thumbs, ok := navSlice(header, "thumbnail", "musicThumbnailRenderer", "thumbnail", "thumbnails"); ok {
		thumbnailURL = bestThumbnail(thumbs)
	}

	p.recover()
	return ArtistMetadata{Name: name, ExtractedID: id, ThumbnailURL: thumbnailURL, Description: description}, nil
}

// State reports YouTube as always available -- its public web endpoints need no credentials.
func (p *youtubeProcessor) State(context.Context) State {
	return State{CanDetect: true, CanLookup: true, CanFetchAlbum: true, CanFetchArtist: true}
}

// Type identifies this processor's source_type.
func (p *youtubeProcessor) Type() SourceType { return SourceTypeYtmusic }

// toSeconds converts a "m:ss" or "h:mm:ss" duration run into whole seconds.
func toSeconds(text string) int32 {
	parts := strings.Split(text, ":")
	var total, mult int32 = 0, 1
	for i := len(parts) - 1; i >= 0; i-- {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return 0
		}
		total += int32(n) * mult
		mult *= 60
	}
	return total
}

// parseSongRuns extracts credited artists, album and duration from a YTM text run list.
func parseSongRuns(runs []map[string]any) (artists []ArtistMetadata, album *AlbumMetadata, durationMs int32) {
	for i, run := range runs {
		if i%2 == 1 {
			continue
		}
		text, _ := run["text"].(string)
		browseID, hasBrowse := runBrowseID(run)
		switch {
		case hasBrowse && (strings.HasPrefix(browseID, "MPRE") || strings.Contains(browseID, "release_detail")):
			album = &AlbumMetadata{Name: text, ExtractedID: browseID}
		case hasBrowse:
			artists = append(artists, ArtistMetadata{Name: text, ExtractedID: browseID})
		case durationPattern.MatchString(text):
			durationMs = toSeconds(text) * 1000
		}
	}
	return
}

// firstSearchVideoID pulls the top hit's video id out of a YTM search response's first music results shelf.
func firstSearchVideoID(searchResp map[string]any) (string, bool) {
	contents, _ := searchResp["contents"].(map[string]any)
	if tabbed, ok := contents["tabbedSearchResultsRenderer"]; ok {
		if c, ok := navMap(tabbed, "tabs", 0, "tabRenderer", "content"); ok {
			contents = c
		}
	}
	sectionList, _ := navSlice(contents, "sectionListRenderer", "contents")
	for _, raw := range sectionList {
		shelf, ok := navMap(raw, "musicShelfRenderer")
		if !ok {
			continue
		}
		shelfContents, _ := navSlice(shelf, "contents")
		for _, entryRaw := range shelfContents {
			item, ok := navMap(entryRaw, "musicResponsiveListItemRenderer")
			if !ok {
				continue
			}
			return navString(item,
				"overlay", "musicItemThumbnailOverlayRenderer", "content", "musicPlayButtonRenderer", "playNavigationEndpoint", "watchEndpoint", "videoId")
		}
	}
	return "", false
}

// bestThumbnail picks the widest thumbnail URL from a YTM thumbnails array.
func bestThumbnail(thumbs []any) string {
	var bestURL string
	var bestWidth float64
	for _, raw := range thumbs {
		t, ok := asMap(raw)
		if !ok {
			continue
		}
		url, _ := t["url"].(string)
		width, _ := t["width"].(float64)
		if url == "" {
			continue
		}
		if bestURL == "" || width > bestWidth {
			bestURL, bestWidth = url, width
		}
	}
	return bestURL
}

// asMap type-asserts v as a JSON object.
func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

// asSlice type-asserts v as a JSON array.
func asSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

// nav walks node through a chain of map keys (string) and slice indices (int), short-circuiting on any miss.
func nav(node any, steps ...any) (any, bool) {
	cur := node
	for _, step := range steps {
		switch s := step.(type) {
		case string:
			m, ok := asMap(cur)
			if !ok {
				return nil, false
			}
			cur, ok = m[s]
			if !ok {
				return nil, false
			}
		case int:
			arr, ok := asSlice(cur)
			if !ok || s < 0 || s >= len(arr) {
				return nil, false
			}
			cur = arr[s]
		default:
			return nil, false
		}
	}
	return cur, true
}

// navMap is nav for a result expected to be a JSON object.
func navMap(node any, steps ...any) (map[string]any, bool) {
	v, ok := nav(node, steps...)
	if !ok {
		return nil, false
	}
	return asMap(v)
}

// navSlice is nav for a result expected to be a JSON array.
func navSlice(node any, steps ...any) ([]any, bool) {
	v, ok := nav(node, steps...)
	if !ok {
		return nil, false
	}
	return asSlice(v)
}

// navString is nav for a result expected to be a string.
func navString(node any, steps ...any) (string, bool) {
	v, ok := nav(node, steps...)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// extractRuns pulls the "runs" array out of a YTM text object ({"runs": [...]} or {"simpleText": ...}).
func extractRuns(node any) []map[string]any {
	rawRuns, ok := navSlice(node, "runs")
	if !ok {
		return nil
	}
	runs := make([]map[string]any, 0, len(rawRuns))
	for _, r := range rawRuns {
		if m, ok := asMap(r); ok {
			runs = append(runs, m)
		}
	}
	return runs
}

// runBrowseID pulls navigationEndpoint.browseEndpoint.browseId out of a single run, if present.
func runBrowseID(run map[string]any) (string, bool) {
	return navString(run, "navigationEndpoint", "browseEndpoint", "browseId")
}
