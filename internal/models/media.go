package models

// MediaKind identifies the normalized type of a library item.
type MediaKind string

const (
	MediaMovie   MediaKind = "movie"
	MediaSeries  MediaKind = "series"
	MediaEpisode MediaKind = "episode"
	MediaArtist  MediaKind = "artist"
	MediaAlbum   MediaKind = "album"
	MediaTrack   MediaKind = "track"
)

// MediaItem is Overmynd's normalized representation of an item managed by
// Radarr, Sonarr, or Lidarr. Service-specific API structures are converted
// into this model before the rest of Overmynd sees them.
type MediaItem struct {
	ID              string      `json:"id"`
	Source          ServiceType `json:"source"`
	SourceServiceID int64       `json:"source_service_id"`
	SourceID        int64       `json:"source_id"`
	Kind            MediaKind   `json:"kind"`
	Title           string      `json:"title"`
	Year            int         `json:"year,omitempty"`
	Path            string      `json:"path,omitempty"`
	Monitored       bool        `json:"monitored"`
	HasFile         bool        `json:"has_file,omitempty"`

	// External identifiers are the strongest cross-service correlation keys.
	TMDBID        int64  `json:"tmdb_id,omitempty"`
	TVDBID        int64  `json:"tvdb_id,omitempty"`
	IMDbID        string `json:"imdb_id,omitempty"`
	MusicBrainzID string `json:"musicbrainz_id,omitempty"`
}
