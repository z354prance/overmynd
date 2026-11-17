package seerr

import (
	"context"
	"strconv"
	"strings"
)

type mediaDetails struct {
	Title string `json:"title"`
	Name  string `json:"name"`
}

func (c *Client) MediaTitle(
	ctx context.Context,
	baseURL string,
	apiKey string,
	mediaType string,
	tmdbID int64,
) (string, error) {
	if tmdbID <= 0 {
		return "", nil
	}

	var path string

	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "movie":
		path = "movie/" + strconv.FormatInt(tmdbID, 10)
	case "tv":
		path = "tv/" + strconv.FormatInt(tmdbID, 10)
	default:
		return "", nil
	}

	var details mediaDetails

	if err := c.GetJSON(
		ctx,
		baseURL,
		apiKey,
		path,
		&details,
	); err != nil {
		return "", err
	}

	if strings.EqualFold(mediaType, "movie") {
		return strings.TrimSpace(details.Title), nil
	}

	return strings.TrimSpace(details.Name), nil
}
