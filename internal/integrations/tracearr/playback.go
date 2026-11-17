package tracearr

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/z354prance/overmynd/internal/models"
)

type streamsResponse struct {
	Streams []streamRecord `json:"data"`
}

type streamRecord struct {
	ID string `json:"id"`

	ServerID   string `json:"server_id"`
	ServerName string `json:"server_name"`
	ServerType string `json:"server_type"`

	Username string `json:"username"`

	MediaTitle    string `json:"media_title"`
	MediaType     string `json:"media_type"`
	ShowTitle     string `json:"show_title"`
	SeasonNumber  int    `json:"season_number"`
	EpisodeNumber int    `json:"episode_number"`
	Year          int    `json:"year"`

	ArtistName  *string `json:"artist_name"`
	AlbumName   *string `json:"album_name"`
	TrackNumber *int    `json:"track_number"`

	DurationMs int64  `json:"duration_ms"`
	ProgressMs int64  `json:"progress_ms"`
	State      string `json:"state"`
	StartedAt  string `json:"started_at"`

	IsTranscode   bool   `json:"is_transcode"`
	VideoDecision string `json:"video_decision"`
	AudioDecision string `json:"audio_decision"`
	Bitrate       int64  `json:"bitrate"`

	Device   string `json:"device"`
	Player   string `json:"player"`
	Product  string `json:"product"`
	Platform string `json:"platform"`

	MediaID     string `json:"media_id"`
	ShowMediaID string `json:"show_media_id"`

	IMDbID string          `json:"imdb_id"`
	TMDBID json.RawMessage `json:"tmdb_id"`
	TVDBID json.RawMessage `json:"tvdb_id"`

	RatingKey            string `json:"rating_key"`
	ParentRatingKey      string `json:"parent_rating_key"`
	GrandparentRatingKey string `json:"grandparent_rating_key"`
	LibraryID            string `json:"library_id"`

	PosterURL string   `json:"poster_url"`
	Genres    []string `json:"genres"`
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func identifierString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}

	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String()
	}

	var integer int64
	if err := json.Unmarshal(raw, &integer); err == nil {
		return strconv.FormatInt(integer, 10)
	}

	return fmt.Sprintf("%s", raw)
}

func (i *Integration) Playback(
	ctx context.Context,
	service models.Service,
	credential string,
) ([]models.PlaybackSession, error) {
	var response streamsResponse

	if err := i.client.GetJSON(
		ctx,
		service.BaseURL,
		"streams",
		credential,
		&response,
	); err != nil {
		return nil, err
	}

	sessions := make(
		[]models.PlaybackSession,
		0,
		len(response.Streams),
	)

	for _, stream := range response.Streams {
		session := models.PlaybackSession{
			ID:              stream.ID,
			Source:          models.ServiceTracearr,
			SourceServiceID: service.ID,

			ServerID:   stream.ServerID,
			ServerName: stream.ServerName,
			ServerType: stream.ServerType,

			Username: stream.Username,

			MediaTitle:    stream.MediaTitle,
			MediaType:     stream.MediaType,
			ShowTitle:     stream.ShowTitle,
			SeasonNumber:  stream.SeasonNumber,
			EpisodeNumber: stream.EpisodeNumber,
			Year:          stream.Year,

			ArtistName:  optionalString(stream.ArtistName),
			AlbumName:   optionalString(stream.AlbumName),
			TrackNumber: optionalInt(stream.TrackNumber),

			DurationMs: stream.DurationMs,
			ProgressMs: stream.ProgressMs,
			State:      stream.State,

			IsTranscode:   stream.IsTranscode,
			VideoDecision: stream.VideoDecision,
			AudioDecision: stream.AudioDecision,
			Bitrate:       stream.Bitrate,

			Device:   stream.Device,
			Player:   stream.Player,
			Product:  stream.Product,
			Platform: stream.Platform,

			MediaID:     stream.MediaID,
			ShowMediaID: stream.ShowMediaID,

			IMDbID: stream.IMDbID,
			TMDBID: identifierString(stream.TMDBID),
			TVDBID: identifierString(stream.TVDBID),

			RatingKey:            stream.RatingKey,
			ParentRatingKey:      stream.ParentRatingKey,
			GrandparentRatingKey: stream.GrandparentRatingKey,
			LibraryID:            stream.LibraryID,

			PosterURL: stream.PosterURL,
			Genres:    stream.Genres,
		}

		if stream.StartedAt != "" {
			if startedAt, err := time.Parse(
				time.RFC3339,
				stream.StartedAt,
			); err == nil {
				session.StartedAt = &startedAt
			}
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}
