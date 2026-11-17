package radarr

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/integrations/arr"
	"github.com/z354prance/overmynd/internal/models"
)

type missingMoviesResponse struct {
	Page         int            `json:"page"`
	PageSize     int            `json:"pageSize"`
	TotalRecords int            `json:"totalRecords"`
	Records      []missingMovie `json:"records"`
}

type missingMovie struct {
	ID              int64  `json:"id"`
	Title           string `json:"title"`
	Year            int    `json:"year"`
	Monitored       bool   `json:"monitored"`
	TMDBID          int64  `json:"tmdbId"`
	IMDbID          string `json:"imdbId"`
	InCinemas       string `json:"inCinemas"`
	DigitalRelease  string `json:"digitalRelease"`
	PhysicalRelease string `json:"physicalRelease"`
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
		"wanted/missing?page=1&pageSize=1000&sortDirection=descending",
	)
	if err != nil {
		return nil, err
	}

	var response missingMoviesResponse

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

	for _, movie := range response.Records {
		if !movie.Monitored {
			continue
		}

		item := models.AttentionItem{
			ID: strconv.FormatInt(
				service.ID,
				10,
			) + ":missing:movie:" + strconv.FormatInt(
				movie.ID,
				10,
			),
			Source:          service.Type,
			SourceServiceID: service.ID,
			SourceID:        movie.ID,
			State:           models.AttentionMissing,
			Kind:            models.MediaMovie,
			Title:           movie.Title,
			Year:            movie.Year,
			Monitored:       movie.Monitored,
			TMDBID:          movie.TMDBID,
			IMDbID:          movie.IMDbID,
		}

		item.AirDate = firstDate(
			movie.DigitalRelease,
			movie.PhysicalRelease,
			movie.InCinemas,
		)

		items = append(items, item)
	}

	return items, nil
}

func firstDate(values ...string) *time.Time {
	for _, value := range values {
		if value == "" {
			continue
		}

		parsed, err := time.Parse(
			time.RFC3339,
			value,
		)
		if err == nil {
			return &parsed
		}
	}

	return nil
}
