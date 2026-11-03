// Package torboxarr reads local transfer progress from TorBoxarr's SABnzbd API.
package torboxarr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type Integration struct{}

func New() *Integration                       { return &Integration{} }
func (*Integration) Type() models.ServiceType { return models.ServiceTorBoxarr }

// SAB accepts read-only modes in form bodies, keeping credentials out of URLs/logs.
func readQueue(ctx context.Context, base, key string) (*queue, error) {
	base, err := integrations.NormalizeBaseURL(base)
	if err != nil || strings.TrimSpace(key) == "" {
		return nil, errors.New("TorBoxarr requires a valid URL and API key")
	}
	endpoint := strings.TrimRight(base, "/")
	if !strings.HasSuffix(endpoint, "/api") {
		endpoint += "/api"
	}
	values := url.Values{"mode": {"queue"}, "output": {"json"}, "apikey": {key}, "limit": {"1000"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, errors.New("unable to create TorBoxarr request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("unable to reach TorBoxarr")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TorBoxarr returned HTTP %d; check its address and API key", resp.StatusCode)
	}
	var result struct {
		Queue *queue `json:"queue"`
	}
	// Do not return upstream errors or raw payloads: they can contain credentials.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&result); err != nil || result.Queue == nil {
		return nil, errors.New("invalid TorBoxarr queue response")
	}
	return result.Queue, nil
}

type queue struct {
	Slots  []slot `json:"slots"`
	Paused bool   `json:"paused"`
}
type slot struct {
	ID         string `json:"nzo_id"`
	Title      string `json:"filename"`
	Status     string `json:"status"`
	MB         number `json:"mb"`
	MBLeft     number `json:"mbleft"`
	Percentage number `json:"percentage"`
	// Optional per-slot speed in KiB/s. Never use the queue-wide speed for each job.
	Kbps     number `json:"kbpersec"`
	TimeLeft string `json:"timeleft"`
}

// SAB-compatible servers may use numbers, numeric strings, or empty values.
type number struct {
	value float64
	valid bool
}

func (n *number) UnmarshalJSON(raw []byte) error {
	text := string(raw)
	if len(raw) > 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	n.valid = err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
	if n.valid {
		n.value = value
	}
	return nil
}
func (*Integration) TestConnection(ctx context.Context, base, key string) (integrations.ConnectionResult, error) {
	_, err := readQueue(ctx, base, key)
	if err != nil {
		return integrations.ConnectionResult{}, err
	}
	return integrations.ConnectionResult{OK: true, Message: "Connected to TorBoxarr local transfer queue"}, nil
}
func (*Integration) Queue(ctx context.Context, service models.Service, key string) ([]models.Download, error) {
	q, err := readQueue(ctx, service.BaseURL, key)
	if err != nil {
		return nil, err
	}
	items := make([]models.Download, 0, len(q.Slots))
	seen := map[string]bool{}
	for _, s := range q.Slots {
		if s.ID == "" || seen[s.ID] {
			continue
		}
		seen[s.ID] = true
		state := strings.ToLower(strings.TrimSpace(s.Status))
		if state == "completed" || state == "deleted" {
			continue
		}
		if state == "" {
			state = "unknown"
		}
		if q.Paused && (state == "downloading" || state == "queued") {
			state = "paused"
		}
		d := models.Download{ID: fmt.Sprintf("%d:torboxarr:%s", service.ID, s.ID), Source: models.ServiceTorBoxarr, SourceServiceID: service.ID, Title: s.Title, Status: state, DownloadClient: service.Name, DownloadID: s.ID, Protocol: "usenet"}
		if s.MB.valid && s.MB.value > 0 && s.MB.value < float64(math.MaxInt64)/(1024*1024) && s.MBLeft.valid {
			d.Size = int64(s.MB.value * 1024 * 1024)
			d.SizeLeft = int64(math.Min(s.MB.value, s.MBLeft.value) * 1024 * 1024)
		} else if s.Percentage.valid && s.Percentage.value <= 100 && !(s.MB.valid && s.MB.value == 0) {
			value := s.Percentage.value
			d.Progress = &value
		}
		if s.Kbps.valid && s.Kbps.value < float64(math.MaxInt64)/1024 && state == "downloading" {
			value := int64(s.Kbps.value * 1024)
			d.Speed = &value
		}
		if s.TimeLeft != "0:00:00" && s.TimeLeft != "00:00:00" && state == "downloading" {
			d.TimeLeft = s.TimeLeft
		}
		items = append(items, d)
	}
	return items, nil
}
