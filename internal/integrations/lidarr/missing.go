package lidarr

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/integrations/arr"
	"github.com/z354prance/overmynd/internal/models"
)

type missingAlbumsResponse struct {
	Page         int            `json:"page"`
	PageSize     int            `json:"pageSize"`
	TotalRecords int            `json:"totalRecords"`
	Records      []missingAlbum `json:"records"`
}

type missingAlbum struct {
	ID             int64  `json:"id"`
	Title          string `json:"title"`
	Monitored      bool   `json:"monitored"`
	ReleaseDate    string `json:"releaseDate"`
	ForeignAlbumID string `json:"foreignAlbumId"`
	Artist         struct {
		ID              int64  `json:"id"`
		ArtistName      string `json:"artistName"`
		ForeignArtistID string `json:"foreignArtistId"`
	} `json:"artist"`
}

func (i *Integration) Missing(
	ctx context.Context,
	service models.Service,
	credential string,
) ([]models.AttentionItem, error) {
	if credential == "" {
		return nil, fmt.Errorf("API key is required")
	}

	client := arr.NewClient("v1")

	endpoint, err := client.Endpoint(
		service.BaseURL,
		"wanted/missing?page=1&pageSize=1000&includeArtist=true&sortDirection=descending",
	)
	if err != nil {
		return nil, err
	}

	var response missingAlbumsResponse

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

	for _, album := range response.Records {
		if !album.Monitored {
			continue
		}

		title := album.Title
		if album.Artist.ArtistName != "" {
			title = album.Artist.ArtistName + " - " + album.Title
		}

		item := models.AttentionItem{
			ID: strconv.FormatInt(service.ID, 10) +
				":missing:album:" +
				strconv.FormatInt(album.ID, 10),
			Source:          service.Type,
			SourceServiceID: service.ID,
			SourceID:        album.ID,
			State:           models.AttentionMissing,
			Kind:            models.MediaAlbum,
			Title:           title,
			Monitored:       album.Monitored,
			MusicBrainzID:   album.ForeignAlbumID,
		}

		if item.MusicBrainzID == "" {
			item.MusicBrainzID = album.Artist.ForeignArtistID
		}

		if album.ReleaseDate != "" {
			parsed, err := time.Parse(
				time.RFC3339,
				album.ReleaseDate,
			)
			if err == nil {
				item.AirDate = &parsed
			}
		}

		items = append(items, item)
	}

	return items, nil
}
