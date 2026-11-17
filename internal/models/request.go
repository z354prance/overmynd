package models

import "time"

type Request struct {
	ID              string      `json:"id"`
	Source          ServiceType `json:"source"`
	SourceServiceID int64       `json:"source_service_id"`
	SourceID        int64       `json:"source_id"`

	MediaType string `json:"media_type"`
	Title     string `json:"title,omitempty"`
	TMDBID    int64  `json:"tmdb_id,omitempty"`

	Status      string `json:"status"`
	MediaStatus string `json:"media_status,omitempty"`

	RequestedBy string `json:"requested_by,omitempty"`

	Seasons []int `json:"seasons,omitempty"`

	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}
