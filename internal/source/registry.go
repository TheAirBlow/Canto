package source

import (
	"context"
	"log/slog"
)

// Registry holds every known Processor, keyed by ID.
type Registry struct {
	processors map[string]Processor
}

// NewRegistry builds a Registry from processors.
func NewRegistry(processors ...Processor) *Registry {
	m := make(map[string]Processor, len(processors))
	for _, p := range processors {
		m[p.ID()] = dedupe(p)
	}
	return &Registry{processors: m}
}

// OrderedLink resolves ids to processors able to detect an origin_url right now in ids order.
func (r *Registry) OrderedLink(ctx context.Context, ids []string) []Processor {
	return r.ordered(ctx, ids, func(s State) bool { return s.CanDetect })
}

// OrderedFallback resolves ids to processors able to look up by text query right now in ids order.
func (r *Registry) OrderedFallback(ctx context.Context, ids []string) []Processor {
	return r.ordered(ctx, ids, func(s State) bool { return s.CanLookup })
}

// IDs returns every registered processor's id.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.processors))
	for id := range r.processors {
		ids = append(ids, id)
	}
	return ids
}

// ByID looks up the registered processor for id.
func (r *Registry) ByID(id string) (Processor, bool) {
	p, ok := r.processors[id]
	return p, ok
}

// ByType looks up the registered processor for sourceType, regardless of configured ordering.
func (r *Registry) ByType(sourceType SourceType) (Processor, bool) {
	for _, p := range r.processors {
		if p.Type() == sourceType {
			return p, true
		}
	}
	return nil, false
}

// ordered resolves ids to registered capable processors in order.
func (r *Registry) ordered(ctx context.Context, ids []string, capable func(State) bool) []Processor {
	out := make([]Processor, 0, len(ids))
	for _, id := range ids {
		p, ok := r.processors[id]
		if !ok {
			slog.Warn("configured processor not registered, skipping", "id", id)
			continue
		}
		if !capable(p.State(ctx)) {
			slog.Warn("configured processor not currently available, skipping", "id", id)
			continue
		}
		if a, ok := p.(Availabler); ok && !a.Available() {
			slog.Warn("configured processor not currently available, skipping", "id", id)
			continue
		}
		out = append(out, p)
	}
	return out
}
