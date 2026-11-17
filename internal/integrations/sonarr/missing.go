package sonarr

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/integrations/arr"
	"github.com/z354prance/overmynd/internal/models"
)

type missingEpisodesResponse struct {
	Page         int              `json:"page"`
	PageSize     int              `json:"pageSize"`
	TotalRecords int              `json:"totalRecords"`
	Records      []missingEpisode `json:"records"`
}

type missingEpisode struct {
	ID            int64  `json:"id"`
	SeriesID      int64  `json:"seriesId"`
	TVDBID        int64  `json:"tvdbId"`
	Title         string `json:"title"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	AirDateUTC    string `json:"airDateUtc"`
	Monitored     bool   `json:"monitored"`
	Series        struct {
		Title  string `json:"title"`
		Year   int    `json:"year"`
		TVDBID int64  `json:"tvdbId"`
		IMDbID string `json:"imdbId"`
	} `json:"series"`
}

func (i *Integration) Missing(
	ctx context.Context,
	service models.Service,
	credential string,
) ([]models.AttentionItem, error) {
	if credential == "" {
		return nil, fmt.Errorf("API key is required")
	}

	client := arr.NewClient("v3")

	endpoint, err := client.Endpoint(
		service.BaseURL,
		"wanted/missing?page=1&pageSize=1000&includeSeries=true&sortDirection=descending",
	)
	if err != nil {
		return nil, err
	}

	var response missingEpisodesResponse

	httpClient := integrations.NewHTTPClient()

	err = httpClient.GetJSON(
		ctx,
		endpoint,
		map[string]string{
			"X-Api-Key": credential,
		},
		&response,
	)
	if err != nil {
		return nil, err
	}

	items := make(
		[]models.AttentionItem,
		0,
		len(response.Records),
	)

	for _, episode := range response.Records {
		if !episode.Monitored {
			continue
		}

		title := episode.Series.Title
		if title == "" {
			title = episode.Title
		}

		item := models.AttentionItem{
			ID: strconv.FormatInt(service.ID, 10) +
				":missing:episode:" +
				strconv.FormatInt(episode.ID, 10),
			Source:          service.Type,
			SourceServiceID: service.ID,
			SourceID:        episode.ID,
			State:           models.AttentionMissing,
			Kind:            models.MediaEpisode,
			Title:           title,
			Year:            episode.Series.Year,
			SeasonNumber:    episode.SeasonNumber,
			EpisodeNumber:   episode.EpisodeNumber,
			Monitored:       episode.Monitored,
			TVDBID:          episode.Series.TVDBID,
			IMDbID:          episode.Series.IMDbID,
		}

		if episode.Series.TVDBID == 0 {
			item.TVDBID = episode.TVDBID
		}

		if episode.AirDateUTC != "" {
			parsed, err := time.Parse(
				time.RFC3339,
				episode.AirDateUTC,
			)
			if err == nil {
				item.AirDate = &parsed
			}
		}

		items = append(items, item)
	}

	return items, nil
}
