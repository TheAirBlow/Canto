package importer

import (
	"context"
	"encoding/json"
	"io"

	"Canto/internal/ingest"
)

// parseJSONArray decodes a top-level JSON array from r and returns its element count immediately, then converts each element at or past startIdx via toListen and streams it to out in the background, closing out when done.
func parseJSONArray[T any](ctx context.Context, r io.Reader, out chan<- ingest.ListenInput, startIdx int, toListen func(T) ingest.ListenInput) (int, error) {
	var items []T
	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return 0, err
	}

	go func() {
		defer close(out)
		for i := startIdx; i < len(items); i++ {
			select {
			case out <- toListen(items[i]):
			case <-ctx.Done():
				return
			}
		}
	}()
	return len(items), nil
}
