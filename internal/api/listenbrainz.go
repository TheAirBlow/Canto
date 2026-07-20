package api

import (
	"log/slog"
	"net/http"
	"slices"
	"time"

	"Canto/internal/auth"
	"Canto/internal/ingest"
)

// listenBrainzIngesterID identifies the ListenBrainz-compatible ingester in enabled/forward settings.
const listenBrainzIngesterID = "listenbrainz"

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
		Normalize:     settings.FuzzyNormalize,
	}
	nowPlaying := req.ListenType == "playing_now"

	for _, item := range req.Payload {
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

		in := ingest.ListenInput{
			UserID:                   user.ID,
			OriginURL:                info.OriginURL,
			ArtistNames:              []string{item.TrackMetadata.ArtistName},
			SongName:                 item.TrackMetadata.TrackName,
			AlbumName:                item.TrackMetadata.ReleaseName,
			ListenedAt:               time.Unix(item.ListenedAt, 0).UTC(),
			DurationMs:               durationMs,
			DurationPlayedMs:         durationPlayedMs,
			SubmissionClient:         info.SubmissionClient,
			SubmissionClientVersion:  info.SubmissionClientVersion,
			OriginalSubmissionClient: info.OriginalSubmissionClient,
			MediaPlayer:              info.MediaPlayer,
			MediaPlayerVersion:       info.MediaPlayerVersion,
			MusicService:             info.MusicService,
			MusicServiceName:         info.MusicServiceName,
		}
		if _, err := s.ingest.SubmitListen(r.Context(), in, procSettings, nowPlaying); err != nil {
			slog.Error("submit listen failed", "user", user.ID, "err", err)
			internalError(w, err.Error())
			return
		}
		s.dispatchForwards(settings.Forwards, listenBrainzIngesterID, req.ListenType, item)
	}

	ok(w, map[string]string{"status": "ok"})
}
