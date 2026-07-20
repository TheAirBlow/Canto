package api

import (
	"net/http"

	"Canto/internal/auth"
)

// registerExport registers the listen-history export endpoint.
func (s *Server) registerExport(mux authMux) {
	mux.CookieAuthHandleFunc("GET /export", s.exportListens)
}

// exportListens returns the caller's full listen history in Canto's own export format.
func (s *Server) exportListens(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())

	export, err := s.export.Export(r.Context(), user.ID)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	ok(w, export)
}
