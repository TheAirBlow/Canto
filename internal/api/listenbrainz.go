package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"Canto/internal/auth"
	"Canto/internal/importer"
	"Canto/internal/ingest"
)

// listenBrainzIngesterID identifies the ListenBrainz-compatible ingester in enabled/forward settings.
const listenBrainzIngesterID = "listenbrainz"

// minValidListenedAt rejects a listen dated before scrobbling meaningfully existed.
var minValidListenedAt = time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC)

// maxFutureSkew tolerates modest client clock drift on a submitted listened_at.
const maxFutureSkew = 5 * time.Minute

// massSubmitThreshold is the payload size past which a submit-listens request is routed through a bulk-import job instead of processed inline.
const massSubmitThreshold = 100

// knownIngesters lists every ingest endpoint Canto exposes, for GET /settings/registry.
var knownIngesters = []registryIngester{
	{ID: listenBrainzIngesterID, Label: "ListenBrainz-compatible", APIPath: "/listenbrainz"},
}

// registerListenBrainz registers the ListenBrainz-compatible submit-listens endpoint.
func (s *Server) registerListenBrainz(mux authMux) {
	mux.TokenAuthHandleFunc("POST /listenbrainz/1/submit-listens", s.submitListens)
}

// lbSubmitRequest is a ListenBrainz submit-listens request body.
type lbSubmitRequest struct {
	ListenType string     `json:"listen_type"`
	Payload    []lbListen `json:"payload"`
}

// lbListen is a single ListenBrainz payload entry.
type lbListen struct {
	ListenedAt    int64           `json:"listened_at"`
	TrackMetadata lbTrackMetadata `json:"track_metadata"`
}

// lbTrackMetadata carries a listen's track/artist/release names.
type lbTrackMetadata struct {
	ArtistName     string           `json:"artist_name"`
	TrackName      string           `json:"track_name"`
	ReleaseName    string           `json:"release_name"`
	AdditionalInfo lbAdditionalInfo `json:"additional_info"`
}

// lbAdditionalInfo carries every extra field Canto reads from a listen submission.
type lbAdditionalInfo struct {
	OriginURL                string `json:"origin_url"`
	RecordingMBID            string `json:"recording_mbid"`
	SpotifyID                string `json:"spotify_id"`
	SubmissionClient         string `json:"submission_client"`
	SubmissionClientVersion  string `json:"submission_client_version"`
	OriginalSubmissionClient string `json:"original_submission_client"`
	MediaPlayer              string `json:"media_player"`
	MediaPlayerVersion       string `json:"media_player_version"`
	MusicService             string `json:"music_service"`
	MusicServiceName         string `json:"music_service_name"`
	DurationMs               *int32 `json:"duration_ms"`
	Duration                 *int32 `json:"duration"`
	DurationPlayed           *int32 `json:"duration_played"`
}

// submitListens records every listen in the payload.
func (s *Server) submitListens(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	settings, err := s.resolveSettings(r.Context(), user.ID)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	if !slices.Contains(settings.Ingesters, listenBrainzIngesterID) {
		http.NotFound(w, r)
		return
	}

	var req lbSubmitRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}

	procSettings := ingest.ProcessorSettings{
		LinkOrder:     settings.LinkProcessors,
		FallbackOrder: settings.FallbackProcessors,
		MatcherOrder:  settings.FuzzyMatchers,
	}
	nowPlaying := req.ListenType == "playing_now"

	entries := make([]ingest.ListenInput, 0, len(req.Payload))
	items := make([]lbListen, 0, len(req.Payload)) // kept parallel to entries, for post-submit forwarding
	for _, item := range req.Payload {
		in, ok := buildListenInput(user.ID, item)
		if !ok {
			slog.Warn("submit listen: rejected implausible listened_at", "user", user.ID, "listened_at", item.ListenedAt)
			continue
		}
		entries = append(entries, in)
		items = append(items, item)
	}

	if !nowPlaying && len(entries) > massSubmitThreshold {
		if err := s.routeToImportBatch(r.Context(), user.ID, entries); err != nil {
			internalError(w, err.Error())
			return
		}
		ok(w, map[string]string{"status": "ok"})
		return
	}

	for i, in := range entries {
		if _, err := s.ingest.SubmitListen(r.Context(), in, procSettings, nowPlaying, false); err != nil {
			slog.Error("submit listen failed", "user", user.ID, "err", err)
			internalError(w, err.Error())
			return
		}
		s.dispatchForwards(settings.Forwards, listenBrainzIngesterID, req.ListenType, items[i])
	}

	ok(w, map[string]string{"status": "ok"})
}

// buildListenInput converts one ListenBrainz payload entry into a ListenInput, reporting false if listened_at is implausible.
func buildListenInput(userID int64, item lbListen) (ingest.ListenInput, bool) {
	listenedAt := time.Unix(item.ListenedAt, 0).UTC()
	if listenedAt.Before(minValidListenedAt) || listenedAt.After(time.Now().Add(maxFutureSkew)) {
		return ingest.ListenInput{}, false
	}

	info := item.TrackMetadata.AdditionalInfo
	var durationPlayedMs int32
	if info.DurationPlayed != nil {
		durationPlayedMs = *info.DurationPlayed
	}

	var durationMs int32
	switch {
	case info.DurationMs != nil:
		durationMs = *info.DurationMs
	case info.Duration != nil:
		durationMs = *info.Duration * 1000
	}

	if info.OriginURL == "" {
		info.OriginURL = ingest.InferOriginURL(info.RecordingMBID, info.SpotifyID)
	}

	return ingest.ListenInput{
		UserID:                   userID,
		OriginURL:                info.OriginURL,
		ArtistNames:              []string{item.TrackMetadata.ArtistName},
		SongName:                 item.TrackMetadata.TrackName,
		AlbumName:                item.TrackMetadata.ReleaseName,
		ListenedAt:               listenedAt,
		DurationMs:               durationMs,
		DurationPlayedMs:         durationPlayedMs,
		SubmissionClient:         info.SubmissionClient,
		SubmissionClientVersion:  info.SubmissionClientVersion,
		OriginalSubmissionClient: info.OriginalSubmissionClient,
		MediaPlayer:              info.MediaPlayer,
		MediaPlayerVersion:       info.MediaPlayerVersion,
		MusicService:             info.MusicService,
		MusicServiceName:         info.MusicServiceName,
	}, true
}

// routeToImportBatch hands entries to the importer as a single job instead of processing them inline.
func (s *Server) routeToImportBatch(ctx context.Context, userID int64, entries []ingest.ListenInput) error {
	data, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal submission batch: %w", err)
	}
	_, err = s.importer.CreateBatch(ctx, userID, importer.ImportServiceIngestBatch, []importer.UploadedFile{
		{Filename: "submission.json", Reader: bytes.NewReader(data)},
	})
	return err
}
