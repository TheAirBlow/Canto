package source

import (
	"context"
	"testing"
	"time"
)

// Rick Astley - Never Gonna Give You Up / Whenever You Need Somebody.
const (
	deezerTestTrackID  = "14408104"
	deezerTestAlbumID  = "901415162"
	deezerTestArtistID = "6160"
)

// TestDeezerFetchMetadata hits the real API and checks every field comes back verbatim.
func TestDeezerFetchMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := NewDeezerProcessor()
	meta, confident, err := p.FetchMetadata(ctx, deezerTestTrackID)
	if err != nil {
		t.Fatalf("FetchMetadata(%s): %v", deezerTestTrackID, err)
	}
	if !confident {
		t.Error("confident = false, want true")
	}

	if meta.SongName != "Never Gonna Give You Up" {
		t.Errorf("SongName = %q, want %q", meta.SongName, "Never Gonna Give You Up")
	}
	if meta.ExtractedID != deezerTestTrackID {
		t.Errorf("ExtractedID = %q, want %q", meta.ExtractedID, deezerTestTrackID)
	}
	if meta.DurationMs != 211000 {
		t.Errorf("DurationMs = %d, want 211000", meta.DurationMs)
	}
	if meta.ThumbnailURL == "" {
		t.Error("ThumbnailURL is empty")
	}

	if len(meta.Artists) != 1 {
		t.Fatalf("len(Artists) = %d, want 1", len(meta.Artists))
	}
	if meta.Artists[0].Name != "Rick Astley" {
		t.Errorf("Artists[0].Name = %q, want %q", meta.Artists[0].Name, "Rick Astley")
	}
	if meta.Artists[0].ExtractedID != deezerTestArtistID {
		t.Errorf("Artists[0].ExtractedID = %q, want %q", meta.Artists[0].ExtractedID, deezerTestArtistID)
	}

	if meta.Album == nil {
		t.Fatal("Album is nil")
	}
	if meta.Album.Name != "Reeling In The Decades" {
		t.Errorf("Album.Name = %q, want %q", meta.Album.Name, "Reeling In The Decades")
	}
}

// TestDeezerFetchAlbum hits the real API and checks the album and its first track come back verbatim.
func TestDeezerFetchAlbum(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := NewDeezerProcessor()
	album, err := p.FetchAlbum(ctx, deezerTestAlbumID)
	if err != nil {
		t.Fatalf("FetchAlbum(%s): %v", deezerTestAlbumID, err)
	}

	if album.Name != "Whenever You Need Somebody" {
		t.Errorf("Name = %q, want %q", album.Name, "Whenever You Need Somebody")
	}
	if album.ExtractedID != deezerTestAlbumID {
		t.Errorf("ExtractedID = %q, want %q", album.ExtractedID, deezerTestAlbumID)
	}
	if album.ThumbnailURL == "" {
		t.Error("ThumbnailURL is empty")
	}
	if len(album.Songs) != 10 {
		t.Fatalf("len(Songs) = %d, want 10", len(album.Songs))
	}

	first := album.Songs[0]
	if first.Name != "Never Gonna Give You Up" {
		t.Errorf("Songs[0].Name = %q, want %q", first.Name, "Never Gonna Give You Up")
	}
	if first.DurationMs != 213000 {
		t.Errorf("Songs[0].DurationMs = %d, want 213000", first.DurationMs)
	}
	if first.TrackNumber != 1 {
		t.Errorf("Songs[0].TrackNumber = %d, want 1", first.TrackNumber)
	}
	if len(first.Artists) != 1 || first.Artists[0].Name != "Rick Astley" {
		t.Errorf("Songs[0].Artists = %+v, want [{Rick Astley ...}]", first.Artists)
	}
	if first.Artists[0].ExtractedID != deezerTestArtistID {
		t.Errorf("Songs[0].Artists[0].ExtractedID = %q, want %q", first.Artists[0].ExtractedID, deezerTestArtistID)
	}

	last := album.Songs[9]
	if last.Name != "When I Fall in Love" {
		t.Errorf("Songs[9].Name = %q, want %q", last.Name, "When I Fall in Love")
	}
	if last.TrackNumber != 10 {
		t.Errorf("Songs[9].TrackNumber = %d, want 10", last.TrackNumber)
	}
}

// TestDeezerFetchArtist hits the real API and checks the artist comes back verbatim.
func TestDeezerFetchArtist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := NewDeezerProcessor()
	artist, err := p.FetchArtist(ctx, deezerTestArtistID)
	if err != nil {
		t.Fatalf("FetchArtist(%s): %v", deezerTestArtistID, err)
	}

	if artist.Name != "Rick Astley" {
		t.Errorf("Name = %q, want %q", artist.Name, "Rick Astley")
	}
	if artist.ExtractedID != deezerTestArtistID {
		t.Errorf("ExtractedID = %q, want %q", artist.ExtractedID, deezerTestArtistID)
	}
	if artist.ThumbnailURL == "" {
		t.Error("ThumbnailURL is empty")
	}
}

// TestDeezerFetchMetadataByQuery hits the real search API and checks the top hit is Rick Astley's track.
func TestDeezerFetchMetadataByQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := NewDeezerProcessor()
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

// TestDeezerDetectAndExtractID covers URL recognition against synthetic inputs, no network involved.
func TestDeezerDetectAndExtractID(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		wantOK bool
		wantID string
	}{
		{"plain track", "https://www.deezer.com/track/14408104", true, "14408104"},
		{"no www", "https://deezer.com/track/14408104", true, "14408104"},
		{"lang segment", "https://www.deezer.com/en/track/14408104", true, "14408104"},
		{"album link", "https://www.deezer.com/album/901415162", false, ""},
		{"wrong host", "https://example.com/track/14408104", false, ""},
		{"non-numeric id", "https://www.deezer.com/track/abc", false, ""},
	}

	p := NewDeezerProcessor()
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
