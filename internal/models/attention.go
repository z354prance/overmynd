package models

import "time"

type AttentionState string

const (
	AttentionMissing    AttentionState = "missing"
	AttentionInProgress AttentionState = "in_progress"
	AttentionProblem    AttentionState = "problem"
)

// AttentionItem represents media that is missing, actively moving through
// the pipeline, or requires attention. Fully available media is excluded.
type AttentionItem struct {
	ID              string         `json:"id"`
	Source          ServiceType    `json:"source"`
	SourceServiceID int64          `json:"source_service_id"`
	SourceID        int64          `json:"source_id"`
	State           AttentionState `json:"state"`
	Kind            MediaKind      `json:"kind"`
	Title           string         `json:"title"`

	Year          int `json:"year,omitempty"`
	SeasonNumber  int `json:"season_number,omitempty"`
	EpisodeNumber int `json:"episode_number,omitempty"`

	Monitored bool `json:"monitored"`

	TMDBID        int64  `json:"tmdb_id,omitempty"`
	TVDBID        int64  `json:"tvdb_id,omitempty"`
	IMDbID        string `json:"imdb_id,omitempty"`
	MusicBrainzID string `json:"musicbrainz_id,omitempty"`

	AirDate *time.Time `json:"air_date,omitempty"`

	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}
