package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/z354prance/undermynd/internal/job"
)

// QBit hands grabs to a local qBittorrent through its WebUI API (v2).
type QBit struct {
	BaseURL  string
	Username string
	Password string
	// SaveDir is the folder (same path for the client and this container) that holds one subfolder per job.
	SaveDir  string
	Category string

	mu     sync.Mutex
	client *http.Client
	authed bool
}

func NewQBit(baseURL, user, pass, saveDir string) *QBit {
	jar, _ := cookiejar.New(nil)
	return &QBit{BaseURL: strings.TrimRight(baseURL, "/"), Username: user, Password: pass, SaveDir: saveDir,
		Category: "undermynd", client: &http.Client{Timeout: 30 * time.Second, Jar: jar}}
}

func (q *QBit) Name() string { return "qbittorrent" }

func (q *QBit) login(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.authed {
		return nil
	}
	form := url.Values{"username": {q.Username}, "password": {q.Password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.BaseURL+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.BaseURL)
	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(string(b)), "Ok") {
		return fmt.Errorf("qbittorrent login refused (HTTP %d)", resp.StatusCode)
	}
	q.authed = true
	return nil
}

func (q *QBit) do(ctx context.Context, method, path string, body io.Reader, ctype string) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if err := q.login(ctx); err != nil {
			return nil, err
		}
		var rdr io.Reader
		if body != nil {
			buf, _ := io.ReadAll(body)
			rdr = bytes.NewReader(buf)
			body = bytes.NewReader(buf) // allow a retry
		}
		req, err := http.NewRequestWithContext(ctx, method, q.BaseURL+path, rdr)
		if err != nil {
			return nil, err
		}
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		req.Header.Set("Referer", q.BaseURL)
		resp, err := q.client.Do(req)
		if err != nil {
			return nil, err
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusForbidden {
			q.mu.Lock()
			q.authed = false
			q.mu.Unlock()
			continue
		}
		if resp.StatusCode >= 300 {
			return b, fmt.Errorf("qbittorrent %s: HTTP %d", path, resp.StatusCode)
		}
		return b, nil
	}
	return nil, fmt.Errorf("qbittorrent %s: not authorised", path)
}

func (q *QBit) Add(ctx context.Context, j job.Job) (string, string, error) {
	save := filepath.Join(q.SaveDir, j.ID)
	if err := os.MkdirAll(save, 0o755); err != nil {
		return "", "", err
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("savepath", save)
	_ = w.WriteField("category", q.Category)
	_ = w.WriteField("autoTMM", "false")
	_ = w.WriteField("sequentialDownload", "false")
	if j.TorrentPath != "" {
		data, err := os.ReadFile(j.TorrentPath)
		if err != nil {
			return "", "", err
		}
		fw, err := w.CreateFormFile("torrents", j.ID+".torrent")
		if err != nil {
			return "", "", err
		}
		_, _ = fw.Write(data)
	} else if j.Magnet != "" {
		_ = w.WriteField("urls", j.Magnet)
	} else {
		return "", "", fmt.Errorf("job has neither a magnet nor a torrent file")
	}
	_ = w.Close()
	out, err := q.do(ctx, http.MethodPost, "/api/v2/torrents/add", &buf, w.FormDataContentType())
	if err != nil {
		return "", "", err
	}
	if strings.Contains(string(out), "Fails") {
		return "", "", fmt.Errorf("qbittorrent rejected the torrent")
	}
	return j.ID, save, nil
}

type qbInfo struct {
	Hash     string  `json:"hash"`
	State    string  `json:"state"`
	Progress float64 `json:"progress"`
}

func (q *QBit) Status(ctx context.Context, id string) (Item, error) {
	b, err := q.do(ctx, http.MethodGet, "/api/v2/torrents/info?hashes="+url.QueryEscape(id), nil, "")
	if err != nil {
		return Item{}, err
	}
	var list []qbInfo
	if err := json.Unmarshal(b, &list); err != nil {
		return Item{}, err
	}
	for _, t := range list {
		if !strings.EqualFold(t.Hash, id) {
			continue
		}
		switch t.State {
		case "error", "missingFiles":
			return Item{Status: Errored, Progress: t.Progress, Error: "qBittorrent state: " + t.State}, nil
		case "checkingDL", "checkingResumeData", "moving":
			return Item{Status: Downloading, Progress: t.Progress}, nil
		}
		if t.Progress >= 1 {
			return Item{Status: Complete, Progress: 1}, nil
		}
		return Item{Status: Downloading, Progress: t.Progress}, nil
	}
	return Item{Status: Missing}, nil
}

func (q *QBit) Files(ctx context.Context, id string) ([]File, error) {
	b, err := q.do(ctx, http.MethodGet, "/api/v2/torrents/files?hash="+url.QueryEscape(id), nil, "")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]File, 0, len(raw))
	for _, f := range raw {
		out = append(out, File{Path: f.Name, Size: f.Size})
	}
	return out, nil
}

func (q *QBit) Remove(ctx context.Context, id string, deleteFiles bool) error {
	form := url.Values{"hashes": {id}, "deleteFiles": {fmt.Sprint(deleteFiles)}}
	_, err := q.do(ctx, http.MethodPost, "/api/v2/torrents/delete", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
	return err
}
