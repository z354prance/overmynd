package qbittorrent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/integrations"
)

type Client struct {
	http *http.Client
}

func NewClient() *Client {
	jar, _ := cookiejar.New(nil)

	return &Client{
		http: &http.Client{
			Timeout: 15 * time.Second,
			Jar:     jar,
		},
	}
}

func (c *Client) Login(
	ctx context.Context,
	baseURL string,
	username string,
	password string,
) error {
	normalized, err := integrations.NormalizeBaseURL(baseURL)
	if err != nil {
		return err
	}

	form := url.Values{}
	form.Set("username", username)
	form.Set("password", password)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		normalized+"/api/v2/auth/login",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return fmt.Errorf("create qBittorrent login request: %w", err)
	}

	req.Header.Set(
		"Content-Type",
		"application/x-www-form-urlencoded",
	)
	req.Header.Set("Referer", normalized)
	req.Header.Set("Origin", normalized)
	req.Header.Set("User-Agent", "Overmynd/dev")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("qBittorrent login: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf(
			"read qBittorrent login response: %w",
			err,
		)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"qBittorrent login returned HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	return nil
}

func (c *Client) GetText(
	ctx context.Context,
	baseURL string,
	path string,
) (string, error) {
	normalized, err := integrations.NormalizeBaseURL(baseURL)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		normalized+path,
		nil,
	)
	if err != nil {
		return "", fmt.Errorf(
			"create qBittorrent request: %w",
			err,
		)
	}

	req.Header.Set("Referer", normalized)
	req.Header.Set("User-Agent", "Overmynd/dev")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("qBittorrent request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf(
			"read qBittorrent response: %w",
			err,
		)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf(
			"qBittorrent returned HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	return strings.TrimSpace(string(body)), nil
}

func (c *Client) GetJSON(
	ctx context.Context,
	baseURL string,
	path string,
	output any,
) error {
	body, err := c.GetText(ctx, baseURL, path)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(body), output); err != nil {
		return fmt.Errorf(
			"decode qBittorrent response: %w",
			err,
		)
	}

	return nil
}
