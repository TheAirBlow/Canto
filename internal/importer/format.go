package importer

import (
	"io"

	"Canto/internal/db"
	"Canto/internal/ingest"
)

// Format translates one bulk-import file format into ListenInput entries.
//
// Parse calls emit once per raw top-level entry it finds, even ones it can't fully parse -- an entry
// emitted with both SongName and OriginURL empty signals a parse failure for that entry (counted as
// skipped, never submitted), so every raw entry is accounted for in the job's progress counters.
type Format interface {
	// ID is this format's import_service discriminator.
	ID() db.ImportService
	// CountEntries does a cheap structural scan of r for the raw top-level entry count.
	CountEntries(r io.Reader) (int, error)
	// Parse streams r, calling emit once per raw top-level entry.
	Parse(r io.Reader, emit func(ingest.ListenInput)) error
}

// defaultFormats returns every built-in Format, keyed by its import_service.
func defaultFormats() map[db.ImportService]Format {
	formats := []Format{
		newCantoExportFormat(),
		newListenBrainzFormat(),
		newSpotifyFormat(),
		newLastFMFormat(),
		newMalojaFormat(),
		newYTMusicFormat(),
	}
	out := make(map[db.ImportService]Format, len(formats))
	for _, f := range formats {
		out[f.ID()] = f
	}
	return out
}
