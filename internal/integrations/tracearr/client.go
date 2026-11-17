package tracearr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func NormalizeBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func (c *Client) GetJSON(
	ctx context.Context,
	baseURL string,
	path string,
	token string,
	result any,
) error {
	endpoint := NormalizeBaseURL(baseURL) +
		"/api/v2/public/" +
		strings.TrimLeft(path, "/")

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint,
		nil,
	)
	if err != nil {
		return fmt.Errorf("Tracearr request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Overmynd")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Tracearr request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(
		io.LimitReader(resp.Body, 10<<20),
	)
	if err != nil {
		return fmt.Errorf("Tracearr response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"Tracearr returned HTTP %d",
			resp.StatusCode,
		)
	}

	if result == nil || len(responseBody) == 0 {
		return nil
	}

	if err := json.Unmarshal(responseBody, result); err != nil {
		return fmt.Errorf("Tracearr JSON: %w", err)
	}

	return nil
}
