package api

import (
	"net/http"

	"github.com/google/uuid"

	"Canto/internal/images"
)

// registerImages registers the cached-image serving endpoint.
func (s *Server) registerImages(mux authMux) {
	mux.HandleFunc("GET /images/{id}/{size}", s.serveImage)
}

// serveImage streams a cached entity image at the requested size, resizing on first request.
func (s *Server) serveImage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		badRequest(w, "invalid id in path")
		return
	}

	var size images.Size
	switch r.PathValue("size") {
	case "small":
		size = images.SizeSmall
	case "medium":
		size = images.SizeMedium
	case "large":
		size = images.SizeLarge
	case "source":
		size = images.SizeSource
	default:
		badRequest(w, "size must be small, medium, large, or source")
		return
	}

	path, err := images.Get(id, size)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
}
