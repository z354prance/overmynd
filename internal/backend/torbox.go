package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/z354prance/overmynd/internal/job"
)

// TorBox downloads usenet items on TorBox and copies the finished files down.
type TorBox struct {
	// Torrents selects the torrent endpoints instead of the usenet ones.
	Torrents bool
	BaseURL  string
	Token    string
	Lim      *Limiter
	Logf     func(format string, args ...any)
	// ListTTL is how long a list result is reused. TorBox refreshes its own list about every 5 seconds.
	ListTTL time.Duration
	Now     func() time.Time

	api *http.Client // API calls
	dl  *http.Client // file transfers (no overall timeout)

	mu       sync.Mutex
	items    map[string]tbItem
	cachedAt time.Time
	seen     map[string]bool // states already logged
}

func NewTorBox(baseURL, token string, lim *Limiter) *TorBox {
	return &TorBox{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, Lim: lim, ListTTL: 10 * time.Second,
		Now: time.Now, api: &http.Client{Timeout: 45 * time.Second}, dl: &http.Client{}, seen: map[string]bool{},
		Logf: func(string, ...any) {}}
}

func (t *TorBox) Name() string { return "torbox" }

// NewTorBoxTorrents is the same adapter pointed at TorBox's torrent endpoints.
func NewTorBoxTorrents(baseURL, token string, lim *Limiter) *TorBox {
	t := NewTorBox(baseURL, token, lim)
	t.Torrents = true
	return t
}

func (t *TorBox) ep(op string) string {
	if t.Torrents {
		return "/v1/api/torrents/" + op
	}
	return "/v1/api/usenet/" + op
}

func (t *TorBox) idField() string {
	if t.Torrents {
		return "torrent_id"
	}
	return "usenet_id"
}

type tbFile struct {
	ID        json.Number `json:"id"`
	Name      string      `json:"name"`
	ShortName string      `json:"short_name"`
	Size      int64       `json:"size"`
}

type tbItem struct {
	ID               json.Number `json:"id"`
	Name             string      `json:"name"`
	Size             int64       `json:"size"`
	Progress         float64     `json:"progress"`
	DownloadState    string      `json:"download_state"`
	DownloadFinished bool        `json:"download_finished"`
	DownloadPresent  bool        `json:"download_present"`
	Active           bool        `json:"active"`
	Files            []tbFile    `json:"files"`
}

type envelope struct {
	Success bool            `json:"success"`
	Error   any             `json:"error"`
	Detail  string          `json:"detail"`
	Data    json.RawMessage `json:"data"`
}

// call performs one API request through the limiter and turns TorBox's
// error shapes into ErrBusy, Permanent, or an ordinary error.
func (t *TorBox) call(ctx context.Context, c Class, method, path string, q url.Values, body io.Reader, ctype string) (json.RawMessage, error) {
	if err := t.Lim.Acquire(ctx, c); err != nil {
		return nil, err
	}
	u := t.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		if c == Create {
			t.Lim.Refund()
		}
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.Token)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := t.api.Do(req)
	if err != nil {
		if c == Create {
			t.Lim.Refund() // never reached TorBox
		}
		return nil, fmt.Errorf("torbox %s: %w", path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode == http.StatusTooManyRequests {
		d := 60 * time.Second
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
			d = time.Duration(s) * time.Second
		}
		t.Lim.Backoff(d)
		return nil, fmt.Errorf("%w: TorBox rate limit (HTTP 429), waiting %s", ErrBusy, d)
	}
	var env envelope
	_ = json.Unmarshal(b, &env)
	code := strings.ToUpper(fmt.Sprint(env.Error))
	switch {
	case strings.Contains(code, "ACTIVE_LIMIT"), strings.Contains(code, "COOLDOWN_LIMIT"):
		t.Lim.Backoff(2 * time.Minute)
		return nil, fmt.Errorf("%w: TorBox says %s", ErrBusy, code)
	case strings.Contains(code, "BOZO_NZB"):
		return nil, Permanent{"TorBox rejected the NZB as invalid"}
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, Permanent{"TorBox refused the API token"}
	case method == http.MethodGet && strings.HasSuffix(path, "mylist") && resp.StatusCode == http.StatusNotFound && strings.Contains(code, "ITEM_NOT_FOUND"):
		// An empty account answers the list with a 404; that is an empty list.
		return json.RawMessage("[]"), nil
	case resp.StatusCode >= 300 || (!env.Success && len(b) > 0 && env.Error != nil):
		return nil, fmt.Errorf("torbox %s: HTTP %d %s %s", path, resp.StatusCode, code, env.Detail)
	}
	return env.Data, nil
}

// Add sends the stored NZB to TorBox. The post_processing field is left out so
// TorBox repairs, unpacks and keeps only the wanted files.
func (t *TorBox) Add(ctx context.Context, j job.Job) (string, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	create := "createusenetdownload"
	if t.Torrents {
		create = "createtorrent"
		_ = w.WriteField("seed", "3") // never seed
		_ = w.WriteField("allow_zip", "false")
		switch {
		case j.Magnet != "":
			_ = w.WriteField("magnet", j.Magnet)
		case j.TorrentPath != "":
			data, err := os.ReadFile(j.TorrentPath)
			if err != nil {
				return "", "", err
			}
			fw, _ := w.CreateFormFile("file", j.Name+".torrent")
			_, _ = fw.Write(data)
		default:
			return "", "", Permanent{"torrent has neither a magnet nor a file"}
		}
		_ = w.WriteField("name", j.Name)
	} else {
		if j.NZBPath == "" {
			return "", "", Permanent{"this job has no NZB"}
		}
		nzb, err := os.ReadFile(j.NZBPath)
		if err != nil {
			return "", "", err
		}
		fw, _ := w.CreateFormFile("file", j.Name+".nzb")
		_, _ = fw.Write(nzb)
		_ = w.WriteField("name", j.Name)
	}
	_ = w.Close()
	data, err := t.call(ctx, Create, http.MethodPost, t.ep(create), nil, &buf, w.FormDataContentType())
	if err != nil {
		return "", "", err
	}
	var out struct {
		Usenet  json.Number `json:"usenetdownload_id"`
		Torrent json.Number `json:"torrent_id"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", "", fmt.Errorf("torbox create: unreadable response")
	}
	id := out.Usenet
	if t.Torrents {
		id = out.Torrent
	}
	if id == "" || id == "0" {
		return "", "", fmt.Errorf("torbox create: no download ID in the response")
	}
	t.invalidate()
	return id.String(), "", nil
}

func (t *TorBox) invalidate() {
	t.mu.Lock()
	t.cachedAt = time.Time{}
	t.mu.Unlock()
}

// list returns every usenet item, reusing a recent result so any number of jobs
// cost one call per interval.
func (t *TorBox) list(ctx context.Context) (map[string]tbItem, error) {
	t.mu.Lock()
	if t.items != nil && t.Now().Sub(t.cachedAt) < t.ListTTL {
		defer t.mu.Unlock()
		return t.items, nil
	}
	t.mu.Unlock()
	// Always ask for live data: TorBox otherwise serves a cached list, which can
	// hide that a download has finished. Calls are already limited to one per ListTTL.
	q := url.Values{"limit": {"1000"}, "bypass_cache": {"true"}}
	data, err := t.call(ctx, API, http.MethodGet, t.ep("mylist"), q, nil, "")
	if err != nil {
		return nil, err
	}
	var list []tbItem
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("torbox list: %w", err)
	}
	m := make(map[string]tbItem, len(list))
	for _, it := range list {
		m[it.ID.String()] = it
	}
	t.mu.Lock()
	t.items, t.cachedAt = m, t.Now()
	t.mu.Unlock()
	return m, nil
}

// lookup asks TorBox for one item by ID, with live data.
func (t *TorBox) lookup(ctx context.Context, id string) (tbItem, bool, error) {
	data, err := t.call(ctx, API, http.MethodGet, t.ep("mylist"), url.Values{"id": {id}, "bypass_cache": {"true"}}, nil, "")
	if err != nil {
		return tbItem{}, false, err
	}
	trim := strings.TrimSpace(string(data))
	if trim == "" || trim == "null" || trim == "[]" || trim == "{}" {
		return tbItem{}, false, nil
	}
	var it tbItem
	if strings.HasPrefix(trim, "[") {
		var list []tbItem
		if err := json.Unmarshal(data, &list); err != nil {
			return tbItem{}, false, fmt.Errorf("torbox lookup: %w", err)
		}
		for _, x := range list {
			if x.ID.String() == id {
				return x, true, nil
			}
		}
		return tbItem{}, false, nil
	}
	if err := json.Unmarshal(data, &it); err != nil {
		return tbItem{}, false, fmt.Errorf("torbox lookup: %w", err)
	}
	if it.ID.String() != id {
		return tbItem{}, false, nil
	}
	return it, true, nil
}

// find returns one item with live data. The account list is capped (and an account
// with many old items can push a new one past the cap or serve it stale), so the
// item is asked for by ID first; the list is only the fallback.
func (t *TorBox) find(ctx context.Context, id string) (tbItem, bool, error) {
	if one, found, err := t.lookup(ctx, id); err == nil && found {
		return one, true, nil
	} else if err != nil && errors.Is(err, ErrBusy) {
		return tbItem{}, false, err
	}
	m, err := t.list(ctx)
	if err != nil {
		return tbItem{}, false, err
	}
	it, ok := m[id]
	if !ok {
		t.Logf("torbox: item %s is not in the account (%d items listed)", id, len(m))
	}
	return it, ok, nil
}

func (t *TorBox) Status(ctx context.Context, id string) (Item, error) {
	it, ok, err := t.find(ctx, id)
	if err != nil {
		return Item{}, err
	}
	if !ok {
		return Item{Status: Missing}, nil
	}
	st := strings.ToLower(it.DownloadState)
	t.mu.Lock()
	key := fmt.Sprintf("%s|%s|%v|%d", id, st, it.DownloadFinished, len(it.Files))
	if !t.seen[key] {
		t.seen[key] = true
		t.Logf("torbox: item %s state %q (progress %v, finished %v, %d files)", id, it.DownloadState, it.Progress, it.DownloadFinished, len(it.Files))
	}
	t.mu.Unlock()
	p := it.Progress
	if p > 1 {
		p /= 100 // the docs do not say whether this is a fraction or a percentage
	}
	for _, bad := range []string{"fail", "error", "incomplete", "missing", "corrupt", "encrypt", "bozo", "dmca", "abort"} {
		if strings.Contains(st, bad) {
			return Item{Status: Errored, Progress: p, Error: "TorBox state: " + it.DownloadState}, nil
		}
	}
	if it.DownloadFinished && len(it.Files) > 0 {
		return Item{Status: Complete, Progress: 1}, nil
	}
	idle := !it.Active && !it.DownloadFinished && !strings.Contains(st, "queue")
	// 100% (or "finished") with no files: TorBox's own unpack or repair has failed or hung.
	stuck := p >= 0.999 || it.DownloadFinished
	return Item{Status: Downloading, Progress: p, Inactive: idle, Stuck: stuck}, nil
}

func (t *TorBox) Files(ctx context.Context, id string) ([]File, error) {
	it, ok, err := t.find(ctx, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("torbox: item not in the list")
	}
	out := make([]File, 0, len(it.Files))
	for _, f := range it.Files {
		out = append(out, File{ID: f.ID.String(), Path: f.Name, Size: f.Size})
	}
	return out, nil
}

// Remove deletes the item and its stored files from the TorBox account.
func (t *TorBox) Remove(ctx context.Context, id string, _ bool) error {
	body, _ := json.Marshal(map[string]any{t.idField(): json.Number(id), "operation": "delete"})
	_, err := t.call(ctx, API, http.MethodPost, t.ep(map[bool]string{true: "controltorrent", false: "controlusenetdownload"}[t.Torrents]), nil, bytes.NewReader(body), "application/json")
	t.invalidate()
	return err
}

// Fetch copies one file to dst through a permanent download link, resuming from
// a partial copy. The link request is paced, and the URL (it carries the token)
// is never logged.
func (t *TorBox) Fetch(ctx context.Context, id string, f File, dst string, progress func(n int64)) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	part := dst + ".part"
	var have int64
	if st, err := os.Stat(part); err == nil {
		have = st.Size()
	}
	if f.Size > 0 && have > f.Size {
		_ = os.Remove(part)
		have = 0
	}
	if f.Size > 0 && have == f.Size {
		return os.Rename(part, dst)
	}
	if err := t.Lim.Acquire(ctx, Link); err != nil {
		return err
	}
	q := url.Values{"token": {t.Token}, t.idField(): {id}, "file_id": {f.ID}, "redirect": {"true"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.BaseURL+t.ep("requestdl")+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := t.dl.Do(req)
	if err != nil {
		return errors.New("torbox download request failed") // do not include the URL
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		d := 60 * time.Second
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
			d = time.Duration(s) * time.Second
		}
		t.Lim.Backoff(d)
		return fmt.Errorf("%w: download link rate limit, waiting %s", ErrBusy, d)
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		return os.Rename(part, dst)
	case resp.StatusCode == http.StatusOK:
		have = 0 // the server ignored Range: start over
	case resp.StatusCode == http.StatusPartialContent:
	default:
		return fmt.Errorf("torbox download: HTTP %d", resp.StatusCode)
	}
	if have > 0 {
		progress(have) // count the bytes already on disk
	}
	flags := os.O_CREATE | os.O_WRONLY
	if have == 0 {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}
	out, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return err
	}
	buf := make([]byte, 256<<10)
	var written int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return werr
			}
			written += int64(n)
			progress(int64(n))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close() // keep the partial file so the next attempt resumes
			return fmt.Errorf("torbox download interrupted: %w", rerr)
		}
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if f.Size > 0 && have+written != f.Size {
		return fmt.Errorf("size mismatch: got %d of %d bytes", have+written, f.Size)
	}
	return os.Rename(part, dst)
}
