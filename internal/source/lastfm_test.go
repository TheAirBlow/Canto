package source

import (
	"context"
	"os"
	"testing"
	"time"
)

// lastFMTestAPIKeyEnv names the env var for a live Last.fm key; these tests skip when it's unset since Last.fm requires a key for every request.
const lastFMTestAPIKeyEnv = "CANTO_TEST_LASTFM_API_KEY"

// newTestLastFMProcessor builds a lastFMProcessor keyed from the environment, skipping t if unset.
func newTestLastFMProcessor(t *testing.T) Processor {
	key := os.Getenv(lastFMTestAPIKeyEnv)
	if key == "" {
		t.Skipf("%s not set, skipping Last.fm live test", lastFMTestAPIKeyEnv)
	}
	return NewLastFMProcessor(key)
}

// TestLastFMFetchMetadataByQuery hits the real API and checks the top hit is Rick Astley's track.
func TestLastFMFetchMetadataByQuery(t *testing.T) {
	p := newTestLastFMProcessor(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	meta, confident, err := p.FetchMetadataByQuery(ctx, Query{Song: "Never Gonna Give You Up", Artist: "Rick Astley"})
	if err != nil {
		t.Fatalf("FetchMetadataByQuery: %v", err)
	}
	if !confident {
		t.Error("confident = false, want true")
	}
	if meta.SongName != "Never Gonna Give You Up" {
		t.Errorf("SongName = %q, want %q", meta.SongName, "Never Gonna Give You Up")
	}
	if len(meta.Artists) == 0 || meta.Artists[0].Name != "Rick Astley" {
		t.Errorf("Artists = %+v, want first entry Rick Astley", meta.Artists)
	}
	if meta.Album == nil || meta.Album.Name == "" {
		t.Errorf("Album = %+v, want a populated album", meta.Album)
	}
}

// TestLastFMFetchArtist hits the real API and checks the artist comes back verbatim.
func TestLastFMFetchArtist(t *testing.T) {
	p := newTestLastFMProcessor(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	artist, err := p.FetchArtist(ctx, "Rick Astley")
	if err != nil {
		t.Fatalf("FetchArtist: %v", err)
	}
	if artist.Name != "Rick Astley" {
		t.Errorf("Name = %q, want %q", artist.Name, "Rick Astley")
	}
	if artist.ExtractedID != "Rick Astley" {
		t.Errorf("ExtractedID = %q, want %q", artist.ExtractedID, "Rick Astley")
	}
	if artist.ThumbnailURL == "" {
		t.Error("ThumbnailURL is empty")
	}
	if artist.Description == "" {
		t.Error("Description is empty")
	}
}

// TestLastFMDetectAndExtractIDUnsupported covers the no-op URL-detection contract, no network involved.
func TestLastFMDetectAndExtractIDUnsupported(t *testing.T) {
	p := NewLastFMProcessor("irrelevant")
	if p.Detect("https://www.last.fm/music/Rick+Astley/_/Never+Gonna+Give+You+Up") {
		t.Error("Detect() = true, want false (Last.fm has no id-based link support)")
	}
	if _, err := p.ExtractID("https://www.last.fm/music/Rick+Astley"); err == nil {
		t.Error("ExtractID() = nil error, want error (Last.fm has no id-based link support)")
	}
}
