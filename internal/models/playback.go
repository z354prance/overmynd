package models

import "time"

type PlaybackSession struct {
	ID              string      `json:"id"`
	Source          ServiceType `json:"source"`
	SourceServiceID int64       `json:"source_service_id"`

	ServerID   string `json:"server_id,omitempty"`
	ServerName string `json:"server_name,omitempty"`
	ServerType string `json:"server_type,omitempty"`

	Username string `json:"username,omitempty"`

	MediaTitle    string `json:"media_title"`
	MediaType     string `json:"media_type,omitempty"`
	ShowTitle     string `json:"show_title,omitempty"`
	SeasonNumber  int    `json:"season_number,omitempty"`
	EpisodeNumber int    `json:"episode_number,omitempty"`
	Year          int    `json:"year,omitempty"`

	ArtistName  string `json:"artist_name,omitempty"`
	AlbumName   string `json:"album_name,omitempty"`
	TrackNumber int    `json:"track_number,omitempty"`

	DurationMs int64      `json:"duration_ms,omitempty"`
	ProgressMs int64      `json:"progress_ms,omitempty"`
	State      string     `json:"state,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`

	IsTranscode   bool   `json:"is_transcode"`
	VideoDecision string `json:"video_decision,omitempty"`
	AudioDecision string `json:"audio_decision,omitempty"`
	Bitrate       int64  `json:"bitrate,omitempty"`

	Device   string `json:"device,omitempty"`
	Player   string `json:"player,omitempty"`
	Product  string `json:"product,omitempty"`
	Platform string `json:"platform,omitempty"`

	MediaID     string `json:"media_id,omitempty"`
	ShowMediaID string `json:"show_media_id,omitempty"`

	IMDbID string `json:"imdb_id,omitempty"`
	TMDBID string `json:"tmdb_id,omitempty"`
	TVDBID string `json:"tvdb_id,omitempty"`

	RatingKey            string `json:"rating_key,omitempty"`
	ParentRatingKey      string `json:"parent_rating_key,omitempty"`
	GrandparentRatingKey string `json:"grandparent_rating_key,omitempty"`
	LibraryID            string `json:"library_id,omitempty"`

	PosterURL string   `json:"poster_url,omitempty"`
	Genres    []string `json:"genres,omitempty"`
}
