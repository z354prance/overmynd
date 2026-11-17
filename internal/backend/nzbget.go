package backend

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/job"
)

// Locator is implemented by backends that decide where the files land only when they finish.
type Locator interface {
	Location(ctx context.Context, remoteID string) (string, error)
}

// NZBGet drives a local NZBGet over its JSON-RPC API. Its finished files are read
// from the shared /data mount, so the NZBGet destination must be the same path in both containers.
type NZBGet struct {
	URL, Username, Password string
	hc                      *http.Client
}

func NewNZBGet(url, user, pass string) *NZBGet {
	return &NZBGet{URL: strings.TrimRight(url, "/"), Username: user, Password: pass, hc: &http.Client{Timeout: 60 * time.Second}}
}

func (n *NZBGet) Name() string { return "nzbget" }

func (n *NZBGet) rpc(ctx context.Context, method string, params ...any) (json.RawMessage, error) {
	if params == nil {
		params = []any{}
	}
	body, _ := json.Marshal(map[string]any{"method": method, "params": params, "id": 1})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL+"/jsonrpc", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if n.Username != "" {
		req.SetBasicAuth(n.Username, n.Password)
	}
	resp, err := n.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nzbget: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, Permanent{"NZBGet refused the username or password"}
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("nzbget %s: HTTP %d, unreadable answer", method, resp.StatusCode)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("nzbget %s: %s", method, out.Error.Message)
	}
	return out.Result, nil
}

func (n *NZBGet) Add(ctx context.Context, j job.Job) (string, string, error) {
	if j.NZBPath == "" {
		return "", "", Permanent{"NZBGet only handles NZBs"}
	}
	nzb, err := os.ReadFile(j.NZBPath)
	if err != nil {
		return "", "", err
	}
	res, err := n.rpc(ctx, "append", j.Name+".nzb", base64.StdEncoding.EncodeToString(nzb), j.Category, 0, false, false, "", 0, "SCORE", []any{})
	if err != nil {
		return "", "", err
	}
	var id int
	if json.Unmarshal(res, &id) != nil || id <= 0 {
		return "", "", errors.New("nzbget did not accept the NZB")
	}
	return strconv.Itoa(id), "", nil
}

type ngGroup struct {
	NZBID       int    `json:"NZBID"`
	Status      string `json:"Status"`
	FileSizeMB  int64  `json:"FileSizeMB"`
	RemainingMB int64  `json:"RemainingSizeMB"`
	DestDir     string `json:"DestDir"`
	FinalDir    string `json:"FinalDir"`
}

func (n *NZBGet) groups(ctx context.Context, method string, params ...any) ([]ngGroup, error) {
	res, err := n.rpc(ctx, method, params...)
	if err != nil {
		return nil, err
	}
	var g []ngGroup
	if err := json.Unmarshal(res, &g); err != nil {
		return nil, err
	}
	return g, nil
}

func (n *NZBGet) find(ctx context.Context, id string) (g ngGroup, inHistory, found bool, err error) {
	want, _ := strconv.Atoi(id)
	q, err := n.groups(ctx, "listgroups", 0)
	if err != nil {
		return g, false, false, err
	}
	for _, x := range q {
		if x.NZBID == want {
			return x, false, true, nil
		}
	}
	h, err := n.groups(ctx, "history", false)
	if err != nil {
		return g, false, false, err
	}
	for _, x := range h {
		if x.NZBID == want {
			return x, true, true, nil
		}
	}
	return g, false, false, nil
}

func (n *NZBGet) Status(ctx context.Context, id string) (Item, error) {
	g, hist, found, err := n.find(ctx, id)
	if err != nil {
		return Item{}, err
	}
	if !found {
		return Item{Status: Missing}, nil
	}
	if !hist {
		p := 0.0
		if g.FileSizeMB > 0 {
			p = 1 - float64(g.RemainingMB)/float64(g.FileSizeMB)
		}
		return Item{Status: Downloading, Progress: p}, nil
	}
	st := strings.ToUpper(g.Status)
	switch {
	case strings.HasPrefix(st, "SUCCESS"):
		return Item{Status: Complete, Progress: 1}, nil
	case strings.HasPrefix(st, "WARNING") && !strings.Contains(st, "HEALTH"):
		return Item{Status: Complete, Progress: 1}, nil
	}
	return Item{Status: Errored, Error: "NZBGet status: " + g.Status}, nil
}

func (n *NZBGet) Location(ctx context.Context, id string) (string, error) {
	g, hist, found, err := n.find(ctx, id)
	if err != nil {
		return "", err
	}
	if !found || !hist {
		return "", errors.New("nzbget: item is not finished")
	}
	if g.FinalDir != "" {
		return g.FinalDir, nil
	}
	return g.DestDir, nil
}

func (n *NZBGet) Files(ctx context.Context, id string) ([]File, error) {
	dir, err := n.Location(ctx, id)
	if err != nil {
		return nil, err
	}
	var out []File
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out = append(out, File{Path: filepath.ToSlash(rel), Size: info.Size()})
		return nil
	})
	return out, err
}

// Remove deletes the item and its files from NZBGet (queue or history).
func (n *NZBGet) Remove(ctx context.Context, id string, _ bool) error {
	_, hist, found, err := n.find(ctx, id)
	if err != nil || !found {
		return err
	}
	cmd := "GroupFinalDelete"
	if hist {
		cmd = "HistoryFinalDelete"
	}
	nid, _ := strconv.Atoi(id)
	_, err = n.rpc(ctx, "editqueue", cmd, "", []int{nid})
	return err
}
