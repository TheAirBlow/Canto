package importer

import (
	"encoding/json"
	"io"
)

// countJSONArray returns the number of top-level elements in a JSON array read from r.
func countJSONArray(r io.Reader) (int, error) {
	dec := json.NewDecoder(r)
	if _, err := dec.Token(); err != nil {
		return 0, err
	}
	var count int
	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

// decodeJSONArray streams a top-level JSON array from r, calling fn with each decoded element.
func decodeJSONArray[T any](r io.Reader, fn func(T)) error {
	dec := json.NewDecoder(r)
	if _, err := dec.Token(); err != nil {
		return err
	}
	for dec.More() {
		var item T
		if err := dec.Decode(&item); err != nil {
			return err
		}
		fn(item)
	}
	return nil
}
