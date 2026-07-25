package importer

import (
	"context"
	"io"

	"Canto/internal/ingest"
)

// ingestBatchFormat parses a JSON array of already-built ingest.ListenInput.
type ingestBatchFormat struct{}

// newIngestBatchFormat builds an ingestBatchFormat.
func newIngestBatchFormat() Format { return ingestBatchFormat{} }

// ID identifies this format's import_service discriminator.
func (ingestBatchFormat) ID() ImportService { return ImportServiceIngestBatch }

// Parse returns the array's total entry count immediately, then streams entries from startIdx onward to out in the background.
func (ingestBatchFormat) Parse(ctx context.Context, r io.Reader, out chan<- ingest.ListenInput, startIdx int) (int, error) {
	return parseJSONArray(ctx, r, out, startIdx, func(e ingest.ListenInput) ingest.ListenInput { return e })
}
