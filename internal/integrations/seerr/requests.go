package seerr

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/models"
)

type requestResponse struct {
	PageInfo requestPageInfo `json:"pageInfo"`
	Results  []requestRecord `json:"results"`
}

type requestPageInfo struct {
	Pages    int `json:"pages"`
	PageSize int `json:"pageSize"`
	Results  int `json:"results"`
	Page     int `json:"page"`
}

type requestRecord struct {
	ID          int64           `json:"id"`
	Status      int             `json:"status"`
	CreatedAt   string          `json:"createdAt"`
	UpdatedAt   string          `json:"updatedAt"`
	Media       requestMedia    `json:"media"`
	RequestedBy requestUser     `json:"requestedBy"`
	Seasons     []requestSeason `json:"seasons"`
}

type requestMedia struct {
	ID        int64  `json:"id"`
	MediaType string `json:"mediaType"`
	TMDBID    int64  `json:"tmdbId"`
	Status    int    `json:"status"`
	Status4K  int    `json:"status4k"`
}

type requestUser struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"displayName"`
	Username    string `json:"username"`
	Email       string `json:"email"`
}

type requestSeason struct {
	ID           int64 `json:"id"`
	SeasonNumber int   `json:"seasonNumber"`
	Status       int   `json:"status"`
}

func (i *Integration) Requests(
	ctx context.Context,
	service models.Service,
	credential string,
) ([]models.Request, error) {
	const pageSize = 100

	client := NewClient()
	requests := make([]models.Request, 0)
	fetched := 0

	for skip := 0; ; skip += pageSize {
		query := url.Values{}
		query.Set("take", strconv.Itoa(pageSize))
		query.Set("skip", strconv.Itoa(skip))
		query.Set("sort", "added")

		var response requestResponse

		err := client.GetJSON(
			ctx,
			service.BaseURL,
			credential,
			"request?"+query.Encode(),
			&response,
		)
		if err != nil {
			return nil, err
		}

		fetched += len(response.Results)

		for _, record := range response.Results {
			status := requestStatus(record.Status)
			availability := mediaStatus(record.Media.Status)

			if !activeRequest(status, availability) {
				continue
			}

			mediaType := strings.ToLower(record.Media.MediaType)

			title, err := client.MediaTitle(
				ctx,
				service.BaseURL,
				credential,
				mediaType,
				record.Media.TMDBID,
			)
			if err != nil {
				title = ""
			}

			item := models.Request{
				ID: strconv.FormatInt(service.ID, 10) +
					":request:" +
					strconv.FormatInt(record.ID, 10),
				Source:          service.Type,
				SourceServiceID: service.ID,
				SourceID:        record.ID,
				MediaType:       mediaType,
				Title:           title,
				TMDBID:          record.Media.TMDBID,
				Status:          requestStatus(record.Status),
				MediaStatus:     mediaStatus(record.Media.Status),
				RequestedBy:     requestUserName(record.RequestedBy),
				CreatedAt:       parseSeerrTime(record.CreatedAt),
				UpdatedAt:       parseSeerrTime(record.UpdatedAt),
			}

			for _, season := range record.Seasons {
				item.Seasons = append(
					item.Seasons,
					season.SeasonNumber,
				)
			}

			requests = append(requests, item)
		}

		if len(response.Results) == 0 {
			break
		}

		if response.PageInfo.Results > 0 {
			if fetched >= response.PageInfo.Results {
				break
			}
			continue
		}

		if len(response.Results) < pageSize {
			break
		}
	}

	return requests, nil
}

func requestStatus(status int) string {
	switch status {
	case 1:
		return "pending"
	case 2:
		return "approved"
	case 3:
		return "declined"
	case 5:
		return "completed"
	default:
		return fmt.Sprintf("unknown:%d", status)
	}
}

func mediaStatus(status int) string {
	switch status {
	case 1:
		return "unknown"
	case 2:
		return "pending"
	case 3:
		return "processing"
	case 4:
		return "partially_available"
	case 5:
		return "available"
	default:
		return fmt.Sprintf("unknown:%d", status)
	}
}

func requestUserName(user requestUser) string {
	if strings.TrimSpace(user.DisplayName) != "" {
		return strings.TrimSpace(user.DisplayName)
	}

	if strings.TrimSpace(user.Username) != "" {
		return strings.TrimSpace(user.Username)
	}

	return ""
}

func parseSeerrTime(value string) *time.Time {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}

	result := parsed.UTC()
	return &result
}

func activeRequest(status, availability string) bool {
	if status == "completed" || status == "declined" {
		return false
	}

	if availability == "available" {
		return false
	}

	return true
}
