package source

import (
	"context"
	"fmt"

	"golang.org/x/sync/singleflight"
)

// dedupedProcessor wraps a Processor so concurrent identical Fetch* calls collapse into one in-flight request, with every other caller sharing its result.
type dedupedProcessor struct {
	Processor
	group singleflight.Group
}

// dedupe wraps p so its Fetch* calls dedupe concurrent identical requests.
func dedupe(p Processor) Processor {
	return &dedupedProcessor{Processor: p}
}

type metadataResult struct {
	meta      Metadata
	confident bool
}

// FetchMetadata dedupes concurrent calls for the same id.
func (d *dedupedProcessor) FetchMetadata(ctx context.Context, id string) (Metadata, bool, error) {
	v, err, _ := d.group.Do("metadata:"+id, func() (any, error) {
		meta, confident, err := d.Processor.FetchMetadata(ctx, id)
		return metadataResult{meta, confident}, err
	})
	if err != nil {
		return Metadata{}, false, err
	}
	r := v.(metadataResult)
	return r.meta, r.confident, nil
}

// FetchMetadataByQuery dedupes concurrent calls for the same query.
func (d *dedupedProcessor) FetchMetadataByQuery(ctx context.Context, q Query) (Metadata, bool, error) {
	key := fmt.Sprintf("query:%s\x00%s\x00%s", q.Artist, q.Song, q.Album)
	v, err, _ := d.group.Do(key, func() (any, error) {
		meta, confident, err := d.Processor.FetchMetadataByQuery(ctx, q)
		return metadataResult{meta, confident}, err
	})
	if err != nil {
		return Metadata{}, false, err
	}
	r := v.(metadataResult)
	return r.meta, r.confident, nil
}

// FetchAlbum dedupes concurrent calls for the same id.
func (d *dedupedProcessor) FetchAlbum(ctx context.Context, id string) (AlbumMetadata, error) {
	v, err, _ := d.group.Do("album:"+id, func() (any, error) {
		return d.Processor.FetchAlbum(ctx, id)
	})
	if err != nil {
		return AlbumMetadata{}, err
	}
	return v.(AlbumMetadata), nil
}

// FetchArtist dedupes concurrent calls for the same id.
func (d *dedupedProcessor) FetchArtist(ctx context.Context, id string) (ArtistMetadata, error) {
	v, err, _ := d.group.Do("artist:"+id, func() (any, error) {
		return d.Processor.FetchArtist(ctx, id)
	})
	if err != nil {
		return ArtistMetadata{}, err
	}
	return v.(ArtistMetadata), nil
}
