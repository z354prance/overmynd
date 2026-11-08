package torboxarr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type TorrentIntegration struct{}

func NewTorrent() *TorrentIntegration                { return &TorrentIntegration{} }
func (*TorrentIntegration) Type() models.ServiceType { return models.ServiceTorBoxarrTorrent }

type torrentSlot struct {
	Hash        string   `json:"hash"`
	Name        string   `json:"name"`
	State       string   `json:"state"`
	Size        int64    `json:"size"`
	Left        *int64   `json:"amount_left"`
	Progress    *float64 `json:"progress"`
	Speed       *int64   `json:"dlspeed"`
	ETA         int64    `json:"eta"`
	ContentPath string   `json:"content_path"`
}

func readTorrents(ctx context.Context, base, credential string) ([]torrentSlot, error) {
	auth, err := integrations.DecodeUsernamePassword(credential)
	if err != nil {
		return nil, errors.New("TorBoxarr torrents require a username and password")
	}
	base, err = integrations.NormalizeBaseURL(base)
	if err != nil {
		return nil, errors.New("invalid TorBoxarr address")
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	call := func(method, path, body string) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, method, base+path, strings.NewReader(body))
		if err != nil {
			return nil, errors.New("invalid TorBoxarr request")
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", base)
		req.Header.Set("Origin", base)
		resp, err := client.Do(req)
		if err != nil {
			return nil, errors.New("unable to reach TorBoxarr torrent interface")
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("TorBoxarr torrent interface returned HTTP %d; check its address and login", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return nil, errors.New("unable to read TorBoxarr response")
		}
		return data, nil
	}
	login, err := call("POST", "/api/v2/auth/login", url.Values{"username": {auth.Username}, "password": {auth.Password}}.Encode())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(login)) != "Ok." {
		return nil, errors.New("TorBoxarr rejected the torrent username or password")
	}
	body, err := call("GET", "/api/v2/torrents/info?filter=all", "")
	if err != nil {
		return nil, err
	}
	var slots []torrentSlot
	if err := json.Unmarshal(body, &slots); err != nil {
		return nil, errors.New("invalid TorBoxarr torrent queue response")
	}
	return slots, nil
}
func (*TorrentIntegration) TestConnection(ctx context.Context, base, credential string) (integrations.ConnectionResult, error) {
	_, err := readTorrents(ctx, base, credential)
	if err != nil {
		return integrations.ConnectionResult{}, err
	}
	return integrations.ConnectionResult{OK: true, Message: "Connected to TorBoxarr torrent transfers"}, nil
}
func (*TorrentIntegration) Queue(ctx context.Context, service models.Service, credential string) ([]models.Download, error) {
	slots, err := readTorrents(ctx, service.BaseURL, credential)
	if err != nil {
		return nil, err
	}
	items := make([]models.Download, 0, len(slots))
	for _, s := range slots {
		status := strings.ToLower(s.State)
		// TorBoxarr uses pausedUP for completed local transfers, not paused downloads.
		switch status {
		case "pausedup", "stoppedup", "uploading", "stalledup", "queuedup", "forcedup":
			continue
		case "queueddl", "metadl", "forcedmetadl":
			status = "queued"
		case "pauseddl", "stoppeddl":
			status = "paused"
		case "checkingresumedata", "checkingdl", "moving":
			status = "verifying"
		case "forceddl":
			status = "downloading"
		case "missingfiles", "error":
			status = "failed"
		}
		if s.Hash == "" {
			continue
		}
		d := models.Download{ID: fmt.Sprintf("%d:torboxarr-torrent:%s", service.ID, s.Hash), Source: models.ServiceTorBoxarrTorrent, SourceServiceID: service.ID, Title: s.Name, Status: status, DownloadClient: service.Name, DownloadID: s.Hash, Protocol: "torrent", OutputPath: s.ContentPath}
		if s.Size > 0 && s.Left != nil {
			d.Size = s.Size
			d.SizeLeft = max(0, min(s.Size, *s.Left))
		} else if s.Progress != nil && *s.Progress >= 0 && *s.Progress <= 1 && status != "queued" {
			v := *s.Progress * 100
			d.Progress = &v
		}
		if status == "downloading" {
			if s.Speed != nil && *s.Speed >= 0 {
				d.Speed = s.Speed
			}
			if s.ETA > 0 && s.ETA < 8640000 {
				d.TimeLeft = (time.Duration(s.ETA) * time.Second).String()
			}
		}
		items = append(items, d)
	}
	return items, nil
}
