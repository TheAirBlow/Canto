package source

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Rick Astley - Never Gonna Give You Up / Whenever You Need Somebody.
const (
	mbTestRecordingID = "8f3471b5-7e6a-48da-86a9-c1c07a0f47ae"
	mbTestReleaseID   = "b02cad0f-c3f1-41f1-80a2-ddcdb25cf9e1"
	mbTestArtistID    = "db92a151-1ac2-438b-bc43-b82e149ddd50"
)

func newTestMusicBrainzProcessor() Processor {
	return NewMusicBrainzProcessor("https://musicbrainz.org/ws/2", 1)
}

// TestMusicBrainzFetchMetadata hits the real API and checks every deterministic field comes back
// verbatim. Which release MusicBrainz reports first for a recording linked to many isn't guaranteed
// stable, so Album's specific title/id isn't asserted -- only that one came back, and that its
// Cover Art Archive URL is derived from that same id.
func TestMusicBrainzFetchMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := newTestMusicBrainzProcessor()
	meta, confident, err := p.FetchMetadata(ctx, mbTestRecordingID)
	if err != nil {
		t.Fatalf("FetchMetadata(%s): %v", mbTestRecordingID, err)
	}
	if !confident {
		t.Error("confident = false, want true")
	}

	if meta.SongName != "Never Gonna Give You Up" {
		t.Errorf("SongName = %q, want %q", meta.SongName, "Never Gonna Give You Up")
	}
	if meta.ExtractedID != mbTestRecordingID {
		t.Errorf("ExtractedID = %q, want %q", meta.ExtractedID, mbTestRecordingID)
	}
	if meta.DurationMs != 212960 {
		t.Errorf("DurationMs = %d, want 212960", meta.DurationMs)
	}

	if len(meta.Artists) != 1 {
		t.Fatalf("len(Artists) = %d, want 1", len(meta.Artists))
	}
	if meta.Artists[0].Name != "Rick Astley" {
		t.Errorf("Artists[0].Name = %q, want %q", meta.Artists[0].Name, "Rick Astley")
	}
	if meta.Artists[0].ExtractedID != mbTestArtistID {
		t.Errorf("Artists[0].ExtractedID = %q, want %q", meta.Artists[0].ExtractedID, mbTestArtistID)
	}

	if meta.Album == nil {
		t.Fatal("Album is nil")
	}
	wantThumbnail := fmt.Sprintf("https://coverartarchive.org/release/%s/front", meta.Album.ExtractedID)
	if meta.Album.ThumbnailURL != wantThumbnail {
		t.Errorf("Album.ThumbnailURL = %q, want %q", meta.Album.ThumbnailURL, wantThumbnail)
	}
}

// TestMusicBrainzFetchMetadataByQuery hits the real API and checks the top hit is Rick Astley's recording.
func TestMusicBrainzFetchMetadataByQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := newTestMusicBrainzProcessor()
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
}

// TestMusicBrainzFetchAlbum hits the real API and checks the release and its full track listing come back verbatim.
func TestMusicBrainzFetchAlbum(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := newTestMusicBrainzProcessor()
	album, err := p.FetchAlbum(ctx, mbTestReleaseID)
	if err != nil {
		t.Fatalf("FetchAlbum(%s): %v", mbTestReleaseID, err)
	}

	if album.Name != "Whenever You Need Somebody" {
		t.Errorf("Name = %q, want %q", album.Name, "Whenever You Need Somebody")
	}
	if album.ExtractedID != mbTestReleaseID {
		t.Errorf("ExtractedID = %q, want %q", album.ExtractedID, mbTestReleaseID)
	}
	wantThumbnail := fmt.Sprintf("https://coverartarchive.org/release/%s/front", mbTestReleaseID)
	if album.ThumbnailURL != wantThumbnail {
		t.Errorf("ThumbnailURL = %q, want %q", album.ThumbnailURL, wantThumbnail)
	}
	if len(album.Songs) != 10 {
		t.Fatalf("len(Songs) = %d, want 10", len(album.Songs))
	}

	first := album.Songs[0]
	if first.Name != "Never Gonna Give You Up" {
		t.Errorf("Songs[0].Name = %q, want %q", first.Name, "Never Gonna Give You Up")
	}
	if first.ExtractedID != mbTestRecordingID {
		t.Errorf("Songs[0].ExtractedID = %q, want %q", first.ExtractedID, mbTestRecordingID)
	}
	if first.DurationMs != 217000 {
		t.Errorf("Songs[0].DurationMs = %d, want 217000", first.DurationMs)
	}
	if first.TrackNumber != 1 {
		t.Errorf("Songs[0].TrackNumber = %d, want 1", first.TrackNumber)
	}
	if len(first.Artists) != 1 || first.Artists[0].Name != "Rick Astley" {
		t.Errorf("Songs[0].Artists = %+v, want [{Rick Astley ...}]", first.Artists)
	}

	last := album.Songs[9]
	if last.Name != "When I Fall in Love" {
		t.Errorf("Songs[9].Name = %q, want %q", last.Name, "When I Fall in Love")
	}
	if last.TrackNumber != 10 {
		t.Errorf("Songs[9].TrackNumber = %d, want 10", last.TrackNumber)
	}
}

// TestMusicBrainzFetchArtist hits the real API and checks the artist comes back verbatim.
func TestMusicBrainzFetchArtist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := newTestMusicBrainzProcessor()
	artist, err := p.FetchArtist(ctx, mbTestArtistID)
	if err != nil {
		t.Fatalf("FetchArtist(%s): %v", mbTestArtistID, err)
	}

	if artist.Name != "Rick Astley" {
		t.Errorf("Name = %q, want %q", artist.Name, "Rick Astley")
	}
	if artist.ExtractedID != mbTestArtistID {
		t.Errorf("ExtractedID = %q, want %q", artist.ExtractedID, mbTestArtistID)
	}
	if artist.Description != "English singer, songwriter and radio personality" {
		t.Errorf("Description = %q, want %q", artist.Description, "English singer, songwriter and radio personality")
	}
}

// TestMusicBrainzDetectAndExtractID covers URL recognition against synthetic inputs, no network involved.
func TestMusicBrainzDetectAndExtractID(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		wantOK bool
		wantID string
	}{
		{"plain recording", "https://musicbrainz.org/recording/8f3471b5-7e6a-48da-86a9-c1c07a0f47ae", true, "8f3471b5-7e6a-48da-86a9-c1c07a0f47ae"},
		{"not a recording link", "https://musicbrainz.org/release/b02cad0f-c3f1-41f1-80a2-ddcdb25cf9e1", false, ""},
		{"malformed mbid", "https://musicbrainz.org/recording/not-a-real-mbid", false, ""},
		{"wrong host", "https://example.com/recording/8f3471b5-7e6a-48da-86a9-c1c07a0f47ae", false, ""},
	}

	p := newTestMusicBrainzProcessor()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.Detect(tc.url); got != tc.wantOK {
				t.Errorf("Detect(%q) = %v, want %v", tc.url, got, tc.wantOK)
			}
			id, err := p.ExtractID(tc.url)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("ExtractID(%q): %v", tc.url, err)
				}
				if id != tc.wantID {
					t.Errorf("ExtractID(%q) = %q, want %q", tc.url, id, tc.wantID)
				}
			} else if err == nil {
				t.Errorf("ExtractID(%q) = %q, want error", tc.url, id)
			}
		})
	}
}
