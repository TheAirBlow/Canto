package importer

import (
	"context"
	"io"

	"Canto/internal/ingest"
)

// ImportService discriminates which bulk-import file format a job was submitted under.
type ImportService string

const (
	ImportServiceSpotify      ImportService = "spotify"
	ImportServiceYtmusic      ImportService = "ytmusic"
	ImportServiceLastfm       ImportService = "lastfm"
	ImportServiceListenbrainz ImportService = "listenbrainz"
	ImportServiceMaloja       ImportService = "maloja"
	ImportServiceCantoExport  ImportService = "canto_export"
	ImportServiceKoito        ImportService = "koito"
	// ImportServiceIngestBatch is an oversized live submission, redirected into a job by the ListenBrainz endpoint itself.
	ImportServiceIngestBatch ImportService = "ingest_batch"
)

// Format translates one bulk-import file format into ListenInput entries.
type Format interface {
	// ID is this format's import_service discriminator.
	ID() ImportService

	// Parse returns r's total entry count immediately, then streams entries from startIdx onward to out in the background, closing out when done.
	Parse(ctx context.Context, r io.Reader, out chan<- ingest.ListenInput, startIdx int) (int, error)
}

// defaultFormats returns every built-in Format, keyed by its import_service.
func defaultFormats() map[ImportService]Format {
	formats := []Format{
		newCantoExportFormat(),
		newListenBrainzFormat(),
		newSpotifyFormat(),
		newLastFMFormat(),
		newMalojaFormat(),
		newYTMusicFormat(),
		newKoitoFormat(),
		newIngestBatchFormat(),
	}
	out := make(map[ImportService]Format, len(formats))
	for _, f := range formats {
		out[f.ID()] = f
	}
	return out
}
