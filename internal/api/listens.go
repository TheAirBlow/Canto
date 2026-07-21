package api

import "time"

// listenerResponse is a listen's or now-playing entry's attributed user, omitted entirely when private.
type listenerResponse struct {
	ID          *int64  `json:"id,omitempty"`
	Username    *string `json:"username,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
	ImageURL    *string `json:"image_url,omitempty"`
}

// listenResponse is one listen of a catalog entity by any user, anonymized if that user is private.
type listenResponse struct {
	ListenedAt time.Time         `json:"listened_at"`
	User       *listenerResponse `json:"user,omitempty"`
}

// listeningNowResponse is one user currently listening to a catalog entity; private users never appear here.
type listeningNowResponse struct {
	User      listenerResponse `json:"user"`
	StartedAt time.Time        `json:"started_at"`
}

// listensPage is a paginated listenResponse list.
type listensPage struct {
	Listens []listenResponse `json:"listens"`
	Total   int64            `json:"total"`
	Page    int              `json:"page"`
	PerPage int              `json:"per_page"`
}
