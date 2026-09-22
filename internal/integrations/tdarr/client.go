package tdarr

import (
	"bytes"
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
	return strings.TrimRight(
		strings.TrimSpace(baseURL),
		"/",
	)
}

func (c *Client) GetJSON(
	ctx context.Context,
	baseURL string,
	path string,
	result any,
) error {
	return c.doJSON(
		ctx,
		http.MethodGet,
		baseURL,
		path,
		nil,
		result,
	)
}

func (c *Client) PostJSON(
	ctx context.Context,
	baseURL string,
	path string,
	payload any,
	result any,
) error {
	return c.doJSON(
		ctx,
		http.MethodPost,
		baseURL,
		path,
		payload,
		result,
	)
}

func (c *Client) doJSON(
	ctx context.Context,
	method string,
	baseURL string,
	path string,
	payload any,
	result any,
) error {
	endpoint := NormalizeBaseURL(baseURL) +
		"/api/v2/" +
		strings.TrimLeft(path, "/")

	var body io.Reader

	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("Tdarr JSON request: %w", err)
		}

		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		method,
		endpoint,
		body,
	)
	if err != nil {
		return fmt.Errorf("Tdarr request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Overmynd")

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Tdarr request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(
		io.LimitReader(resp.Body, 10<<20),
	)
	if err != nil {
		return fmt.Errorf("Tdarr response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"Tdarr returned HTTP %d",
			resp.StatusCode,
		)
	}

	if result == nil || len(responseBody) == 0 {
		return nil
	}

	if err := json.Unmarshal(responseBody, result); err != nil {
		return fmt.Errorf("Tdarr JSON: %w", err)
	}

	return nil
}
