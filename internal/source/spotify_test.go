package source

import (
	"context"
	"testing"
	"time"
)

// Rick Astley - Never Gonna Give You Up / Whenever You Need Somebody.
const (
	spotifyTestTrackID  = "4uLU6hMCjMI75M1A2tKUQC"
	spotifyTestAlbumID  = "6N9PS4QXF1D0OWPk0Sxtb4"
	spotifyTestArtistID = "0gxyHStUsqpMadRV0Di1Qt"
)

// A single credited to four artists, used to test multi-artist parsing.
const (
	spotifyMultiArtistTrackID = "4PXV0EHRDBaorVWDhJmaDG"
	spotifyMultiArtistAlbumID = "7r1h1VLCDptixuegbcCmIJ"
)

var spotifyMultiArtistWant = []struct {
	name string
	id   string
}{
	{"まどろみ幽園劇団", "1er4ZRM7WPILTB5LBQOJUw"},
	{"桃寝ちのい", "1sa0AnPAjbscZMgLuSN1gV"},
	{"日あさ寝", "6tAugp1drpsrh2b5Hf68d0"},
	{"いなみこ", "3ZG7wMQ9LEivvg3nd4FwRI"},
}

func checkSpotifyArtists(t *testing.T, got []ArtistMetadata, want []struct {
	name string
	id   string
}) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(Artists) = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Name != w.name {
			t.Errorf("Artists[%d].Name = %q, want %q", i, got[i].Name, w.name)
		}
		if got[i].ExtractedID != w.id {
			t.Errorf("Artists[%d].ExtractedID = %q, want %q", i, got[i].ExtractedID, w.id)
		}
	}
}

// TestSpotifyFetchMetadata hits the real pathfinder getTrack query and checks every field, including the album.
func TestSpotifyFetchMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := NewSpotifyProcessor()
	meta, confident, err := p.FetchMetadata(ctx, spotifyTestTrackID)
	if err != nil {
		t.Fatalf("FetchMetadata(%s): %v", spotifyTestTrackID, err)
	}
	if !confident {
		t.Error("confident = false, want true")
	}

	if meta.SongName != "Never Gonna Give You Up" {
		t.Errorf("SongName = %q, want %q", meta.SongName, "Never Gonna Give You Up")
	}
	if meta.ExtractedID != spotifyTestTrackID {
		t.Errorf("ExtractedID = %q, want %q", meta.ExtractedID, spotifyTestTrackID)
	}
	if meta.DurationMs != 213573 {
		t.Errorf("DurationMs = %d, want 213573", meta.DurationMs)
	}
	if meta.ThumbnailURL == "" {
		t.Error("ThumbnailURL is empty")
	}

	if meta.Album == nil {
		t.Fatal("Album is nil, want Whenever You Need Somebody")
	}
	if meta.Album.Name != "Whenever You Need Somebody" {
		t.Errorf("Album.Name = %q, want %q", meta.Album.Name, "Whenever You Need Somebody")
	}
	if meta.Album.ExtractedID != spotifyTestAlbumID {
		t.Errorf("Album.ExtractedID = %q, want %q", meta.Album.ExtractedID, spotifyTestAlbumID)
	}
	if meta.Album.ThumbnailURL == "" {
		t.Error("Album.ThumbnailURL is empty")
	}

	if len(meta.Artists) != 1 {
		t.Fatalf("len(Artists) = %d, want 1", len(meta.Artists))
	}
	if meta.Artists[0].Name != "Rick Astley" {
		t.Errorf("Artists[0].Name = %q, want %q", meta.Artists[0].Name, "Rick Astley")
	}
	if meta.Artists[0].ExtractedID != spotifyTestArtistID {
		t.Errorf("Artists[0].ExtractedID = %q, want %q", meta.Artists[0].ExtractedID, spotifyTestArtistID)
	}
}

// TestSpotifyFetchMetadataMultiArtist checks all four credited artists come back with real name and id.
func TestSpotifyFetchMetadataMultiArtist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := NewSpotifyProcessor()
	meta, confident, err := p.FetchMetadata(ctx, spotifyMultiArtistTrackID)
	if err != nil {
		t.Fatalf("FetchMetadata(%s): %v", spotifyMultiArtistTrackID, err)
	}
	if !confident {
		t.Error("confident = false, want true")
	}

	if meta.SongName != "おいでよ電波幽園" {
		t.Errorf("SongName = %q, want %q", meta.SongName, "おいでよ電波幽園")
	}
	if meta.ExtractedID != spotifyMultiArtistTrackID {
		t.Errorf("ExtractedID = %q, want %q", meta.ExtractedID, spotifyMultiArtistTrackID)
	}
	if meta.DurationMs != 207202 {
		t.Errorf("DurationMs = %d, want 207202", meta.DurationMs)
	}
	if meta.ThumbnailURL == "" {
		t.Error("ThumbnailURL is empty")
	}

	if meta.Album == nil {
		t.Fatal("Album is nil")
	}
	if meta.Album.Name != "おいでよ電波幽園" {
		t.Errorf("Album.Name = %q, want %q", meta.Album.Name, "おいでよ電波幽園")
	}
	if meta.Album.ExtractedID != spotifyMultiArtistAlbumID {
		t.Errorf("Album.ExtractedID = %q, want %q", meta.Album.ExtractedID, spotifyMultiArtistAlbumID)
	}

	checkSpotifyArtists(t, meta.Artists, spotifyMultiArtistWant)
}

// TestSpotifyFetchMetadataByQuery checks the top search hit is Rick Astley's track, including its album.
func TestSpotifyFetchMetadataByQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := NewSpotifyProcessor()
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
	if meta.ExtractedID == "" {
		t.Error("ExtractedID is empty")
	}
	if meta.DurationMs <= 0 {
		t.Errorf("DurationMs = %d, want > 0", meta.DurationMs)
	}
	if len(meta.Artists) == 0 || meta.Artists[0].Name != "Rick Astley" {
		t.Errorf("Artists = %+v, want first entry Rick Astley", meta.Artists)
	}
	if meta.Artists[0].ExtractedID != spotifyTestArtistID {
		t.Errorf("Artists[0].ExtractedID = %q, want %q", meta.Artists[0].ExtractedID, spotifyTestArtistID)
	}
	if meta.Album == nil || meta.Album.Name != "Whenever You Need Somebody" {
		t.Errorf("Album = %+v, want name %q", meta.Album, "Whenever You Need Somebody")
	}
}

// TestSpotifyFetchAlbum checks the album and its first track come back verbatim.
func TestSpotifyFetchAlbum(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := NewSpotifyProcessor()
	album, err := p.FetchAlbum(ctx, spotifyTestAlbumID)
	if err != nil {
		t.Fatalf("FetchAlbum(%s): %v", spotifyTestAlbumID, err)
	}

	if album.Name != "Whenever You Need Somebody" {
		t.Errorf("Name = %q, want %q", album.Name, "Whenever You Need Somebody")
	}
	if album.ExtractedID != spotifyTestAlbumID {
		t.Errorf("ExtractedID = %q, want %q", album.ExtractedID, spotifyTestAlbumID)
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
	if first.ExtractedID != spotifyTestTrackID {
		t.Errorf("Songs[0].ExtractedID = %q, want %q", first.ExtractedID, spotifyTestTrackID)
	}
	if first.DurationMs != 213573 {
		t.Errorf("Songs[0].DurationMs = %d, want 213573", first.DurationMs)
	}
	if first.TrackNumber != 1 {
		t.Errorf("Songs[0].TrackNumber = %d, want 1", first.TrackNumber)
	}
	if len(first.Artists) != 1 || first.Artists[0].Name != "Rick Astley" {
		t.Errorf("Songs[0].Artists = %+v, want [{Rick Astley}]", first.Artists)
	}
	if first.Artists[0].ExtractedID != spotifyTestArtistID {
		t.Errorf("Songs[0].Artists[0].ExtractedID = %q, want %q", first.Artists[0].ExtractedID, spotifyTestArtistID)
	}
}

// TestSpotifyFetchAlbumMultiArtist checks the multi-artist single's one track carries all four artists with real ids.
func TestSpotifyFetchAlbumMultiArtist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := NewSpotifyProcessor()
	album, err := p.FetchAlbum(ctx, spotifyMultiArtistAlbumID)
	if err != nil {
		t.Fatalf("FetchAlbum(%s): %v", spotifyMultiArtistAlbumID, err)
	}

	if album.Name != "おいでよ電波幽園" {
		t.Errorf("Name = %q, want %q", album.Name, "おいでよ電波幽園")
	}
	if album.ExtractedID != spotifyMultiArtistAlbumID {
		t.Errorf("ExtractedID = %q, want %q", album.ExtractedID, spotifyMultiArtistAlbumID)
	}
	if len(album.Songs) != 1 {
		t.Fatalf("len(Songs) = %d, want 1", len(album.Songs))
	}

	track := album.Songs[0]
	if track.ExtractedID != spotifyMultiArtistTrackID {
		t.Errorf("Songs[0].ExtractedID = %q, want %q", track.ExtractedID, spotifyMultiArtistTrackID)
	}
	checkSpotifyArtists(t, track.Artists, spotifyMultiArtistWant)
}

// TestSpotifyFetchArtist hits the real pathfinder queryArtistOverview query and checks every field, including a non-empty biography.
func TestSpotifyFetchArtist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p := NewSpotifyProcessor()
	artist, err := p.FetchArtist(ctx, spotifyTestArtistID)
	if err != nil {
		t.Fatalf("FetchArtist(%s): %v", spotifyTestArtistID, err)
	}

	if artist.Name != "Rick Astley" {
		t.Errorf("Name = %q, want %q", artist.Name, "Rick Astley")
	}
	if artist.ExtractedID != spotifyTestArtistID {
		t.Errorf("ExtractedID = %q, want %q", artist.ExtractedID, spotifyTestArtistID)
	}
	if artist.ThumbnailURL == "" {
		t.Error("ThumbnailURL is empty")
	}
	if artist.Description == "" {
		t.Error("Description is empty")
	}
}

// TestSpotifyDetectAndExtractID covers URL recognition against synthetic inputs, no network involved.
func TestSpotifyDetectAndExtractID(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		wantOK bool
		wantID string
	}{
		{"plain track", "https://open.spotify.com/track/4uLU6hMCjMI75M1A2tKUQC", true, "4uLU6hMCjMI75M1A2tKUQC"},
		{"www prefix", "https://www.open.spotify.com/track/4uLU6hMCjMI75M1A2tKUQC", true, "4uLU6hMCjMI75M1A2tKUQC"},
		{"with query", "https://open.spotify.com/track/4uLU6hMCjMI75M1A2tKUQC?si=abc123", true, "4uLU6hMCjMI75M1A2tKUQC"},
		{"intl locale prefix", "https://open.spotify.com/intl-de/track/4uLU6hMCjMI75M1A2tKUQC", true, "4uLU6hMCjMI75M1A2tKUQC"},
		{"album link", "https://open.spotify.com/album/6N9PS4QXF1D0OWPk0Sxtb4", false, ""},
		{"wrong host", "https://example.com/track/4uLU6hMCjMI75M1A2tKUQC", false, ""},
		{"malformed id", "https://open.spotify.com/track/not-an-id", false, ""},
	}

	p := NewSpotifyProcessor()
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

// TestSpotifyIDFromURI covers the "spotify:<kind>:<id>" URI parser against synthetic inputs.
func TestSpotifyIDFromURI(t *testing.T) {
	cases := []struct {
		uri  string
		want string
	}{
		{"spotify:artist:0gxyHStUsqpMadRV0Di1Qt", "0gxyHStUsqpMadRV0Di1Qt"},
		{"spotify:track:4uLU6hMCjMI75M1A2tKUQC", "4uLU6hMCjMI75M1A2tKUQC"},
		{"not-a-uri", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := spotifyIDFromURI(tc.uri); got != tc.want {
			t.Errorf("spotifyIDFromURI(%q) = %q, want %q", tc.uri, got, tc.want)
		}
	}
}
