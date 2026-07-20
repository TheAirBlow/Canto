package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"Canto/internal/db"
)

// cascadeQueueSize bounds how many pending rename cascades Client buffers before dropping new ones.
const cascadeQueueSize = 256

// Client talks to a Meilisearch instance.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	queries    *db.Queries
	cascades   chan cascadeJob
}

// NewClient builds a Client against baseURL, using queries for cascaded re-indexing lookups.
func NewClient(baseURL, apiKey string, queries *db.Queries) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second}, queries: queries,
		cascades: make(chan cascadeJob, cascadeQueueSize),
	}
}

// Enabled reports whether a Meilisearch instance is configured.
func (c *Client) Enabled() bool {
	return c != nil && c.baseURL != ""
}

// indexes lists every entity-type index this client maintains.
var indexes = []string{"artists", "albums", "songs"}

// Document is one catalog entity as indexed in Meilisearch.
type Document struct {
	ID             int64    `json:"id"`
	EntityType     string   `json:"entity_type"`
	Name           string   `json:"name"`
	NameNormalized string   `json:"name_normalized"`
	ArtistIDs      []int64  `json:"artist_ids,omitempty"`
	ArtistNames    []string `json:"artist_names,omitempty"`
	AlbumID        *int64   `json:"album_id,omitempty"`
	AlbumName      string   `json:"album_name,omitempty"`
}

// EnsureIndexes idempotently creates every entity-type index with id as its primary key.
func (c *Client) EnsureIndexes(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	for _, index := range indexes {
		body, err := json.Marshal(map[string]string{"uid": index, "primaryKey": "id"})
		if err != nil {
			return fmt.Errorf("search: marshal create index %s: %w", index, err)
		}
		req, err := c.request(ctx, http.MethodPost, "/indexes", body)
		if err != nil {
			return err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("search: create index %s: %w", index, err)
		}
		resp.Body.Close()
		// A 409 here means the index already exists, which is the common case after the first run.
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusConflict {
			return fmt.Errorf("search: create index %s: status %d", index, resp.StatusCode)
		}
	}
	return nil
}

// Upsert writes doc into entityType's index, logging and swallowing any failure so indexing never blocks correlation.
func (c *Client) Upsert(ctx context.Context, entityType string, doc Document) {
	if !c.Enabled() {
		return
	}
	body, err := json.Marshal([]Document{doc})
	if err != nil {
		slog.Warn("search: marshal upsert failed, skipping", "entity", entityType, "id", doc.ID, "err", err)
		return
	}
	req, err := c.request(ctx, http.MethodPut, "/indexes/"+entityType+"/documents", body)
	if err != nil {
		slog.Warn("search: build upsert request failed, skipping", "entity", entityType, "id", doc.ID, "err", err)
		return
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Warn("search: upsert failed, skipping", "entity", entityType, "id", doc.ID, "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.Warn("search: upsert failed, skipping", "entity", entityType, "id", doc.ID, "status", resp.StatusCode)
	}
}

// Delete removes id from entityType's index, logging and swallowing any failure.
func (c *Client) Delete(ctx context.Context, entityType string, id int64) {
	if !c.Enabled() {
		return
	}
	req, err := c.request(ctx, http.MethodDelete, fmt.Sprintf("/indexes/%s/documents/%d", entityType, id), nil)
	if err != nil {
		slog.Warn("search: build delete request failed, skipping", "entity", entityType, "id", id, "err", err)
		return
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Warn("search: delete failed, skipping", "entity", entityType, "id", id, "err", err)
		return
	}
	resp.Body.Close()
}

// Hit is one search result document with its relevance-ranked position preserved via slice order.
type Hit struct {
	ID int64 `json:"id"`
}

// searchResponse is the subset of a Meilisearch search response Canto needs.
type searchResponse struct {
	Hits []Hit `json:"hits"`
}

// Search queries entityType's index for q, optionally scoped by a Meilisearch filter expression, returning up to limit hits.
func (c *Client) Search(ctx context.Context, entityType, q, filter string, limit int) ([]Hit, error) {
	if !c.Enabled() {
		return nil, nil
	}
	query := map[string]any{"q": q, "limit": limit}
	if filter != "" {
		query["filter"] = filter
	}
	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("search: marshal query: %w", err)
	}
	req, err := c.request(ctx, http.MethodPost, "/indexes/"+entityType+"/search", body)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search: query %s: %w", entityType, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search: query %s: status %d", entityType, resp.StatusCode)
	}

	var res searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("search: decode query %s: %w", entityType, err)
	}
	return res.Hits, nil
}

// reindexBatchSize bounds how many rows Reindex fetches and re-upserts per page.
const reindexBatchSize = 500

// Reindex rebuilds every index from queries, walking the full catalog in id order.
func (c *Client) Reindex(ctx context.Context, queries *db.Queries) error {
	if !c.Enabled() {
		return nil
	}

	var artistCount, albumCount, songCount int

	var after int64
	for {
		rows, err := queries.ListArtists(ctx, db.ListArtistsParams{After: after, MaxRows: reindexBatchSize})
		if err != nil {
			return fmt.Errorf("search: reindex list artists: %w", err)
		}
		for _, row := range rows {
			c.Upsert(ctx, "artists", Document{ID: row.ID, EntityType: "artist", Name: row.Name, NameNormalized: row.NameNormalized})
			after = row.ID
		}
		artistCount += len(rows)
		if len(rows) < reindexBatchSize {
			break
		}
	}

	after = 0
	for {
		rows, err := queries.ListAlbums(ctx, db.ListAlbumsParams{After: after, MaxRows: reindexBatchSize})
		if err != nil {
			return fmt.Errorf("search: reindex list albums: %w", err)
		}
		for _, row := range rows {
			doc, err := buildAlbumDocument(ctx, queries, row)
			if err != nil {
				return err
			}
			c.Upsert(ctx, "albums", doc)
			after = row.ID
		}
		albumCount += len(rows)
		if len(rows) < reindexBatchSize {
			break
		}
	}

	after = 0
	for {
		rows, err := queries.ListSongs(ctx, db.ListSongsParams{After: after, MaxRows: reindexBatchSize})
		if err != nil {
			return fmt.Errorf("search: reindex list songs: %w", err)
		}
		for _, row := range rows {
			doc, err := buildSongDocument(ctx, queries, row)
			if err != nil {
				return err
			}
			c.Upsert(ctx, "songs", doc)
			after = row.ID
		}
		songCount += len(rows)
		if len(rows) < reindexBatchSize {
			break
		}
	}

	slog.Info("search: reindex complete", "artists", artistCount, "albums", albumCount, "songs", songCount)
	return nil
}

// buildAlbumDocument assembles album's search Document, including its current linked artists.
func buildAlbumDocument(ctx context.Context, queries *db.Queries, album db.Album) (Document, error) {
	artists, err := queries.ListArtistsForAlbum(ctx, album.ID)
	if err != nil {
		return Document{}, fmt.Errorf("search: album %d artists: %w", album.ID, err)
	}
	ids := make([]int64, len(artists))
	names := make([]string, len(artists))
	for i, a := range artists {
		ids[i], names[i] = a.ID, a.Name
	}
	return Document{
		ID: album.ID, EntityType: "album", Name: album.Name, NameNormalized: album.NameNormalized,
		ArtistIDs: ids, ArtistNames: names,
	}, nil
}

// buildSongDocument assembles song's search Document, including its current linked artists and album.
func buildSongDocument(ctx context.Context, queries *db.Queries, song db.Song) (Document, error) {
	artists, err := queries.ListArtistsForSong(ctx, song.ID)
	if err != nil {
		return Document{}, fmt.Errorf("search: song %d artists: %w", song.ID, err)
	}
	ids := make([]int64, len(artists))
	names := make([]string, len(artists))
	for i, a := range artists {
		ids[i], names[i] = a.ID, a.Name
	}
	var albumID *int64
	var albumName string
	if album, err := queries.GetAlbumForSong(ctx, song.ID); err == nil {
		albumID, albumName = &album.ID, album.Name
	}
	return Document{
		ID: song.ID, EntityType: "song", Name: song.Name, NameNormalized: song.NameNormalized,
		ArtistIDs: ids, ArtistNames: names, AlbumID: albumID, AlbumName: albumName,
	}, nil
}

// cascadeTimeout bounds each queued cascade job's processing, independent of the request that triggered it.
const cascadeTimeout = 2 * time.Minute

// cascadeJob is one queued rename cascade, drained and processed sequentially by Run.
type cascadeJob struct {
	entityType string // "artist" or "album"
	id         int64
}

// CascadeArtistRename queues a re-upsert of every album/song linked to artistID with its new name.
func (c *Client) CascadeArtistRename(artistID int64) {
	c.enqueueCascade(cascadeJob{entityType: "artist", id: artistID})
}

// CascadeAlbumRename queues a re-upsert of every song linked to albumID with its new album name.
func (c *Client) CascadeAlbumRename(albumID int64) {
	c.enqueueCascade(cascadeJob{entityType: "album", id: albumID})
}

// enqueueCascade queues job without blocking the caller, dropping and logging it if the queue is full.
func (c *Client) enqueueCascade(job cascadeJob) {
	if !c.Enabled() {
		return
	}
	select {
	case c.cascades <- job:
	default:
		slog.Warn("search: cascade queue full, dropping job", "entity", job.entityType, "id", job.id)
	}
}

// Run drains queued rename cascades one at a time until ctx is canceled. Call in its own goroutine.
func (c *Client) Run(ctx context.Context) {
	if !c.Enabled() {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-c.cascades:
			c.processCascade(ctx, job)
		}
	}
}

// processCascade runs one queued job to completion, bounded by its own timeout derived from ctx.
func (c *Client) processCascade(ctx context.Context, job cascadeJob) {
	jobCtx, cancel := context.WithTimeout(ctx, cascadeTimeout)
	defer cancel()

	switch job.entityType {
	case "artist":
		c.cascadeArtistRename(jobCtx, job.id)
	case "album":
		c.cascadeAlbumRename(jobCtx, job.id)
	}
}

// cascadeArtistRename is CascadeArtistRename's processing body.
func (c *Client) cascadeArtistRename(ctx context.Context, artistID int64) {
	albums, err := c.queries.ListAlbumsForArtist(ctx, artistID)
	if err != nil {
		slog.Warn("search: cascade artist rename: list albums failed", "artist", artistID, "err", err)
		return
	}
	for _, album := range albums {
		if doc, err := buildAlbumDocument(ctx, c.queries, album); err != nil {
			slog.Warn("search: cascade artist rename: build album doc failed", "artist", artistID, "album", album.ID, "err", err)
		} else {
			c.Upsert(ctx, "albums", doc)
		}
	}

	songs, err := c.queries.ListSongsForArtist(ctx, artistID)
	if err != nil {
		slog.Warn("search: cascade artist rename: list songs failed", "artist", artistID, "err", err)
		return
	}
	for _, song := range songs {
		if doc, err := buildSongDocument(ctx, c.queries, song); err != nil {
			slog.Warn("search: cascade artist rename: build song doc failed", "artist", artistID, "song", song.ID, "err", err)
		} else {
			c.Upsert(ctx, "songs", doc)
		}
	}

	slog.Info("search: cascaded artist rename", "artist", artistID, "albums", len(albums), "songs", len(songs))
}

// cascadeAlbumRename is CascadeAlbumRename's processing body.
func (c *Client) cascadeAlbumRename(ctx context.Context, albumID int64) {
	tracks, err := c.queries.ListSongsForAlbum(ctx, albumID)
	if err != nil {
		slog.Warn("search: cascade album rename: list songs failed", "album", albumID, "err", err)
		return
	}
	for _, track := range tracks {
		song := db.Song{
			ID: track.ID, Name: track.Name, NameNormalized: track.NameNormalized,
			DurationMs: track.DurationMs, ImageID: track.ImageID, Pinned: track.Pinned,
			CreatedAt: track.CreatedAt, UpdatedAt: track.UpdatedAt,
		}
		if doc, err := buildSongDocument(ctx, c.queries, song); err != nil {
			slog.Warn("search: cascade album rename: build song doc failed", "album", albumID, "song", song.ID, "err", err)
		} else {
			c.Upsert(ctx, "songs", doc)
		}
	}

	slog.Info("search: cascaded album rename", "album", albumID, "songs", len(tracks))
}

// request builds an authenticated request against path, with body as the JSON payload when non-nil.
func (c *Client) request(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("search: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	return req, nil
}
