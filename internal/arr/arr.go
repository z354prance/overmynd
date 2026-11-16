// Package arr talks to Radarr, Sonarr and Lidarr to confirm imports.
package arr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	Kind    string
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	note    string
}

// Note says what the last history check saw, so a job that never gets confirmed can be explained.
func (c *Client) Note() string { return c.note }

func New(kind, baseURL, apiKey string) *Client {
	return &Client{Kind: kind, BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey,
		HTTP: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) prefix() string {
	if c.Kind == "lidarr" {
		return "/api/v1"
	}
	return "/api/v3"
}

type historyResp struct {
	Records []struct {
		EventType  string `json:"eventType"`
		DownloadID string `json:"downloadId"`
	} `json:"records"`
}

// Imported reports whether the arr's history shows an import for the download ID.
func (c *Client) Imported(ctx context.Context, downloadID string) (bool, error) {
	q := url.Values{}
	q.Set("page", "1")
	q.Set("pageSize", "100")
	q.Set("downloadId", downloadID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+c.prefix()+"/history?"+q.Encode(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("X-Api-Key", c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("%s history: HTTP %d", c.Kind, resp.StatusCode)
	}
	var h historyResp
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return false, err
	}
	var seen []string
	matched := 0
	for _, r := range h.Records {
		if !strings.EqualFold(r.DownloadID, downloadID) {
			continue // the endpoint may ignore the filter on older versions
		}
		matched++
		seen = append(seen, r.EventType)
		e := strings.ToLower(r.EventType)
		if strings.Contains(e, "imported") && !strings.Contains(e, "fail") {
			return true, nil
		}
	}
	c.note = fmt.Sprintf("%s history returned %d record(s), %d for this download (events: %s)", c.Kind, len(h.Records), matched, strings.Join(seen, ", "))
	return false, nil
}

// Check proves the address and key work by reading the arr's status, and returns
// its name and version. It also reads the history, the call the import check
// makes, so a key that can read status but not history is caught too.
func (c *Client) Check(ctx context.Context) (name, version string, err error) {
	get := func(path string, out any) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+c.prefix()+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-Api-Key", c.APIKey)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return fmt.Errorf("%s refused the API key (HTTP %d)", c.Kind, resp.StatusCode)
		case resp.StatusCode != http.StatusOK:
			return fmt.Errorf("%s answered HTTP %d for %s", c.Kind, resp.StatusCode, path)
		}
		if out == nil {
			return nil
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}
	var st struct {
		AppName string `json:"appName"`
		Version string `json:"version"`
	}
	if err = get("/system/status", &st); err != nil {
		return "", "", err
	}
	if err = get("/history?page=1&pageSize=1", nil); err != nil {
		return "", "", err
	}
	return st.AppName, st.Version, nil
}

// FailDownload tells the arr a download failed: it finds the queue entry for the
// download ID and removes it with blocklist=true, so the arr blocklists the release
// and searches for a different one. It reports false when the arr has no such entry
// (it already dealt with it).
func (c *Client) FailDownload(ctx context.Context, downloadID string) (bool, error) {
	do := func(method, path string, out any) (int, error) {
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+c.prefix()+path, nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("X-Api-Key", c.APIKey)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		if out != nil && resp.StatusCode == http.StatusOK {
			return resp.StatusCode, json.NewDecoder(resp.Body).Decode(out)
		}
		return resp.StatusCode, nil
	}
	var q struct {
		Records []struct {
			ID         int64  `json:"id"`
			DownloadID string `json:"downloadId"`
		} `json:"records"`
	}
	code, err := do(http.MethodGet, "/queue?page=1&pageSize=1000", &q)
	if err != nil {
		return false, err
	}
	if code != http.StatusOK {
		return false, fmt.Errorf("%s queue: HTTP %d", c.Kind, code)
	}
	found := false
	for _, r := range q.Records {
		if !strings.EqualFold(r.DownloadID, downloadID) {
			continue
		}
		found = true
		code, err := do(http.MethodDelete, fmt.Sprintf("/queue/%d?removeFromClient=true&blocklist=true&skipRedownload=false", r.ID), nil)
		if err != nil {
			return false, err
		}
		if code != http.StatusOK && code != http.StatusNoContent {
			return false, fmt.Errorf("%s queue delete: HTTP %d", c.Kind, code)
		}
	}
	return found, nil
}
