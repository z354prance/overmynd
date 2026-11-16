// Package tdarr is the only place that knows Tdarr's HTTP API. The calls are the
// ones Overmynd already uses against a live Tdarr (cruddb for file records, get-nodes
// for workers) plus scan-files. Every call is read-only except the scan, and every
// query is for one specific file path, so other libraries are never touched.
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
	BaseURL string
	APIKey  string // sent as x-api-key when Tdarr has authentication on
	HTTP    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), APIKey: apiKey, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) do(ctx context.Context, method, path string, payload, out any) error {
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/api/v2/"+strings.TrimLeft(path, "/"), body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.APIKey != "" {
		req.Header.Set("x-api-key", c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("tdarr: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("tdarr refused the API key (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("tdarr returned HTTP %d", resp.StatusCode)
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("tdarr: unreadable response: %w", err)
	}
	return nil
}

// FileRecord is the part of a FileJSONDB record we rely on.
type FileRecord struct {
	ID          string `json:"_id"` // the file's path as Tdarr sees it
	DB          string `json:"DB"`  // library ID
	HealthCheck string `json:"HealthCheck"`
	Transcode   string `json:"TranscodeDecisionMaker"`
	HoldUntil   int64  `json:"holdUntil"`
}

// Record returns the record for one path, or nil when Tdarr has not seen the file yet.
func (c *Client) Record(ctx context.Context, path string) (*FileRecord, error) {
	req := map[string]any{"data": map[string]any{"collection": "FileJSONDB", "mode": "getById", "docID": path, "obj": map[string]any{}}}
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodPost, "cruddb", req, &raw); err != nil {
		return nil, err
	}
	t := strings.TrimSpace(string(raw))
	if t == "" || t == "null" || t == "{}" || t == "false" || t == `""` {
		return nil, nil
	}
	var r FileRecord
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("tdarr: unreadable file record: %w", err)
	}
	if r.ID == "" {
		return nil, nil
	}
	return &r, nil
}

// Worker is a worker that is currently holding a file.
type Worker struct {
	Node       string
	Worker     string
	File       string
	Percentage float64
	Status     string
	ETA        string
	FPS        float64
}

type nodeRec struct {
	ID       string               `json:"_id"`
	NodeName string               `json:"nodeName"`
	Workers  map[string]workerRec `json:"workers"`
}

type workerRec struct {
	ID         string  `json:"_id"`
	Idle       bool    `json:"idle"`
	File       string  `json:"file"`
	Percentage float64 `json:"percentage"`
	FPS        float64 `json:"fps"`
	ETA        string  `json:"ETA"`
	Status     string  `json:"status"`
}

// ActiveWorkers lists every busy worker, so the caller can pick out its own files by path.
func (c *Client) ActiveWorkers(ctx context.Context) ([]Worker, error) {
	var nodes map[string]nodeRec
	if err := c.do(ctx, http.MethodGet, "get-nodes", nil, &nodes); err != nil {
		return nil, err
	}
	var out []Worker
	for k, n := range nodes {
		name := n.NodeName
		if name == "" {
			name = k
		}
		for wk, w := range n.Workers {
			if w.Idle || strings.TrimSpace(w.File) == "" {
				continue
			}
			id := w.ID
			if id == "" {
				id = wk
			}
			p := w.Percentage
			if p < 0 {
				p = 0
			}
			if p > 100 {
				p = 100
			}
			out = append(out, Worker{Node: name, Worker: id, File: w.File, Percentage: p, Status: w.Status, ETA: w.ETA, FPS: w.FPS})
		}
	}
	return out, nil
}

// Scan asks Tdarr to look for new files in the library's folder, the same request
// its "Scan (find new)" button sends, so a new file is picked up at once and does not
// depend on a folder watcher or a schedule. The path is a single folder string; a list
// of paths is read as individual files and finds nothing in a folder.
func (c *Client) Scan(ctx context.Context, libraryID, path string) error {
	req := map[string]any{"data": map[string]any{"scanConfig": map[string]any{"dbID": libraryID, "arrayOrPath": path, "mode": "scanFindNew"}}}
	return c.do(ctx, http.MethodPost, "scan-files", req, nil)
}

// Check proves the address and key work, returning how many nodes and busy workers there are.
func (c *Client) Check(ctx context.Context) (nodes, busy int, err error) {
	var m map[string]nodeRec
	if err = c.do(ctx, http.MethodGet, "get-nodes", nil, &m); err != nil {
		return 0, 0, err
	}
	w, _ := c.ActiveWorkers(ctx)
	return len(m), len(w), nil
}

// Failed reports whether a record's status words mean Tdarr gave up on the file.
func Failed(r *FileRecord) bool {
	for _, s := range []string{r.Transcode, r.HealthCheck} {
		l := strings.ToLower(s)
		if strings.Contains(l, "error") || strings.Contains(l, "fail") || strings.Contains(l, "cancel") || strings.Contains(l, "unhealthy") {
			return true
		}
	}
	return false
}

// NotRequired reports whether Tdarr decided the file needs no work.
func NotRequired(r *FileRecord) bool {
	return strings.Contains(strings.ToLower(r.Transcode), "not required")
}

// Succeeded reports whether Tdarr marked the file as transcoded.
func Succeeded(r *FileRecord) bool {
	return strings.Contains(strings.ToLower(r.Transcode), "success")
}
