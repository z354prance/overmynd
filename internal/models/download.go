package models

import "time"

// Download represents a normalized download or ARR queue item.
//
// Source identifies the application reporting the item. Fields that do not
// apply to a particular source remain empty rather than leaking
// service-specific structures into the rest of Overmynd.
type Download struct {
	ID               string      `json:"id"`
	Source           ServiceType `json:"source"`
	SourceServiceID  int64       `json:"source_service_id"`
	Title            string      `json:"title"`
	Status           string      `json:"status"`
	TrackedDownload  string      `json:"tracked_download_status,omitempty"`
	Protocol         string      `json:"protocol,omitempty"`
	DownloadClient   string      `json:"download_client,omitempty"`
	DownloadID       string      `json:"download_id,omitempty"`
	OutputPath       string      `json:"output_path,omitempty"`
	Size             int64       `json:"size,omitempty"`
	SizeLeft         int64       `json:"size_left,omitempty"`
	TimeLeft         string      `json:"time_left,omitempty"`
	EstimatedArrival *time.Time  `json:"estimated_arrival,omitempty"`

	// Media references are populated when the originating ARR application
	// supplies them. Their meaning depends on Source.
	MovieID   int64 `json:"movie_id,omitempty"`
	SeriesID  int64 `json:"series_id,omitempty"`
	EpisodeID int64 `json:"episode_id,omitempty"`
	ArtistID  int64 `json:"artist_id,omitempty"`
	AlbumID   int64 `json:"album_id,omitempty"`
}
