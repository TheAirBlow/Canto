package source

import (
	"context"
	"testing"
	"time"
)

// TestYouTubeFetchMetadata checks two known videos come back fully populated, confident, and consistent.
func TestYouTubeFetchMetadata(t *testing.T) {
	cases := []struct {
		name string
		id   string
	}{
		{"video1", "SuP_gK4q2a8"},
		{"video2", "vnT3MKo80hg"},
	}

	p := NewYouTubeProcessor()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			meta, confident, err := p.FetchMetadata(ctx, tc.id)
			if err != nil {
				t.Fatalf("FetchMetadata(%s): %v", tc.id, err)
			}
			if !confident {
				t.Errorf("FetchMetadata(%s) confident = false, want true", tc.id)
			}

			if meta.SongName == "" {
				t.Error("SongName is empty")
			}
			if meta.ExtractedID != tc.id {
				t.Errorf("ExtractedID = %q, want %q", meta.ExtractedID, tc.id)
			}
			if meta.DurationMs <= 0 {
				t.Errorf("DurationMs = %d, want > 0", meta.DurationMs)
			}
			if meta.ThumbnailURL == "" {
				t.Error("ThumbnailURL is empty")
			}
			if len(meta.Artists) == 0 {
				t.Error("Artists is empty")
			}
			for i, a := range meta.Artists {
				if a.Name == "" {
					t.Errorf("Artists[%d].Name is empty", i)
				}
			}

			t.Logf("id=%s song=%q duration=%dms artists=%+v album=%+v",
				tc.id, meta.SongName, meta.DurationMs, meta.Artists, meta.Album)
		})
	}
}

// TestYouTubeFetchAlbum hits the real YTM API for a known album and checks its track listing resolves.
func TestYouTubeFetchAlbum(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := NewYouTubeProcessor()
	album, err := p.FetchAlbum(ctx, "MPREb_OBCnFXaT2Ml")
	if err != nil {
		t.Fatalf("FetchAlbum: %v", err)
	}
	if album.Name == "" {
		t.Error("album Name is empty")
	}
	if len(album.Songs) == 0 {
		t.Error("album Songs is empty")
	}
	for i, track := range album.Songs {
		if track.Name == "" || track.ExtractedID == "" {
			t.Errorf("album.Songs[%d] incomplete: %+v", i, track)
		}
	}

	t.Logf("album=%q tracks=%d songs=%+v", album.Name, len(album.Songs), album.Songs)
}

// TestYouTubeFetchArtist hits the real YTM API for a known artist and checks profile metadata resolves.
func TestYouTubeFetchArtist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := NewYouTubeProcessor()
	artist, err := p.FetchArtist(ctx, "UCmaMCoqM95x-g28nEv257Ew")
	if err != nil {
		t.Fatalf("FetchArtist: %v", err)
	}
	if artist.Name == "" {
		t.Error("artist Name is empty")
	}

	t.Logf("artist=%+v", artist)
}

// TestToSeconds covers the "m:ss" / "h:mm:ss" duration parsing against synthetic inputs.
func TestToSeconds(t *testing.T) {
	cases := []struct {
		text string
		want int32
	}{
		{"3:45", 225},
		{"1:02:03", 3723},
		{"0:09", 9},
	}
	for _, tc := range cases {
		if got := toSeconds(tc.text); got != tc.want {
			t.Errorf("toSeconds(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}
