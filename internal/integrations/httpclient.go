package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseSize = 10 << 20

type HTTPClient struct {
	client *http.Client
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func NormalizeBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimRight(value, "/")

	if value == "" {
		return "", fmt.Errorf("base URL is required")
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf(
			"base URL must use http or https",
		)
	}

	if parsed.Host == "" {
		return "", fmt.Errorf("base URL requires a host")
	}

	return value, nil
}

func (c *HTTPClient) GetJSON(
	ctx context.Context,
	endpoint string,
	headers map[string]string,
	target any,
) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint,
		nil,
	)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Overmynd/dev")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(
			io.LimitReader(resp.Body, 4096),
		)

		message := strings.TrimSpace(string(body))

		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}

		return fmt.Errorf(
			"remote service returned %d: %s",
			resp.StatusCode,
			message,
		)
	}

	reader := io.LimitReader(resp.Body, maxResponseSize)

	if err := json.NewDecoder(reader).Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}
