package seerr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/integrations"
)

type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) GetJSON(
	ctx context.Context,
	baseURL string,
	apiKey string,
	path string,
	result any,
) error {
	normalized, err := integrations.NormalizeBaseURL(baseURL)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		normalized+"/api/v1/"+strings.TrimPrefix(path, "/"),
		nil,
	)
	if err != nil {
		return fmt.Errorf("create Seerr request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("User-Agent", "Overmynd/dev")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Seerr request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return fmt.Errorf("read Seerr response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"Seerr returned HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	if result == nil {
		return nil
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("decode Seerr response: %w", err)
	}

	return nil
}
