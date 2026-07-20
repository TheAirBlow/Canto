package importer

import (
	"encoding/json"
	"io"

	"Canto/internal/db"
	"Canto/internal/export"
	"Canto/internal/ingest"
)

// cantoExportFormat parses Canto's own export format, produced by internal/export.
type cantoExportFormat struct{}

// newCantoExportFormat builds a cantoExportFormat.
func newCantoExportFormat() Format { return cantoExportFormat{} }

// ID identifies this format's import_service discriminator.
func (cantoExportFormat) ID() db.ImportService { return db.ImportServiceCantoExport }

// CountEntries does a cheap structural scan of r for the raw top-level entry count.
func (f cantoExportFormat) CountEntries(r io.Reader) (int, error) {
	doc, err := f.decode(r)
	if err != nil {
		return 0, err
	}
	return len(doc.Listens), nil
}

// Parse streams r, calling emit once per raw top-level entry.
func (f cantoExportFormat) Parse(r io.Reader, emit func(ingest.ListenInput)) error {
	doc, err := f.decode(r)
	if err != nil {
		return err
	}
	for _, l := range doc.Listens {
		song, ok := doc.Songs[l.SongID]
		if !ok {
			emit(ingest.ListenInput{OriginalSubmissionClient: "canto_export"})
			continue
		}

		artistNames := make([]string, 0, len(song.ArtistIDs))
		for _, artistID := range song.ArtistIDs {
			if artist, ok := doc.Artists[artistID]; ok {
				artistNames = append(artistNames, artist.Name)
			}
		}
		var albumName string
		if song.AlbumID != nil {
			if album, ok := doc.Albums[*song.AlbumID]; ok {
				albumName = album.Name
			}
		}
		var durationMs int32
		if song.DurationMs != nil {
			durationMs = *song.DurationMs
		}
		var originURL string
		for _, s := range song.Sources {
			if s.RawURL != nil && *s.RawURL != "" {
				originURL = *s.RawURL
				break
			}
		}

		emit(ingest.ListenInput{
			OriginalSubmissionClient: "canto_export",
			SubmissionClient:         l.Client,
			OriginURL:                originURL,
			ArtistNames:              artistNames,
			SongName:                 song.Name,
			AlbumName:                albumName,
			DurationMs:               durationMs,
			ListenedAt:               l.ListenedAt,
		})
	}
	return nil
}

// decode fully decodes r as an export.Export document.
func (cantoExportFormat) decode(r io.Reader) (export.Export, error) {
	var doc export.Export
	err := json.NewDecoder(r).Decode(&doc)
	return doc, err
}
