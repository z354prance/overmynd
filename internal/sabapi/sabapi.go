// Package sabapi answers the SABnzbd API calls Sonarr, Radarr and Lidarr make, from the job table.
package sabapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/z354prance/undermynd/internal/config"
	"github.com/z354prance/undermynd/internal/job"
	"github.com/z354prance/undermynd/internal/pipeline"
	"github.com/z354prance/undermynd/internal/store"
)

type API struct {
	Store  *store.Store
	Engine *pipeline.Engine
	Cfg    *config.Config
	Log    *log.Logger
	// Fetch downloads an NZB from a URL; replaced in tests.
	Fetch func(ctx context.Context, u string) ([]byte, string, error)
}

func New(s *store.Store, e *pipeline.Engine, c *config.Config, l *log.Logger) *API {
	return &API{Store: s, Engine: e, Cfg: c, Log: l, Fetch: fetchURL}
}

// Handler serves /api and /sabnzbd/api. The API key is the client password
// (or the username and password pair), the same credentials as the qBittorrent side.
func (a *API) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := strings.TrimSuffix(r.URL.Path, "/"); p != "/api" && p != "/sabnzbd/api" {
			http.NotFound(w, r)
			return
		}
		a.serve(w, r)
	})
}

func (a *API) authorised(r *http.Request) bool {
	s := a.Cfg.Get()
	eq := func(x, y string) bool { return subtle.ConstantTimeCompare([]byte(x), []byte(y)) == 1 }
	pw := string(s.ClientPassword)
	if k := r.FormValue("apikey"); k != "" && eq(k, pw) {
		return true
	}
	if k := r.FormValue("ma_password"); r.FormValue("ma_username") != "" && eq(r.FormValue("ma_username"), s.ClientUser) && eq(k, pw) {
		return true
	}
	return false
}

func (a *API) serve(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		_ = r.ParseForm()
	}
	mode := r.FormValue("mode")
	// SABnzbd answers version without a key; everything else needs it.
	if mode == "version" {
		writeJSON(w, map[string]any{"version": "4.3.3"})
		return
	}
	if !a.authorised(r) {
		time.Sleep(500 * time.Millisecond)
		writeJSON(w, map[string]any{"status": false, "error": "API Key Incorrect"})
		return
	}
	switch mode {
	case "auth":
		writeJSON(w, map[string]any{"auth": "apikey"})
	case "get_config":
		a.getConfig(w)
	case "get_cats":
		cats := []string{"*"}
		for _, c := range a.Cfg.Get().Categories {
			cats = append(cats, c.Name)
		}
		writeJSON(w, map[string]any{"categories": cats})
	case "get_scripts":
		writeJSON(w, map[string]any{"scripts": []string{"None"}})
	case "queue":
		a.queue(w, r)
	case "history":
		a.history(w, r)
	case "addurl":
		a.addURL(w, r)
	case "addfile", "addlocalfile":
		a.addFile(w, r)
	case "fullstatus", "status":
		writeJSON(w, map[string]any{"status": map[string]any{"version": "4.3.3", "paused": false}})
	case "pause", "resume", "set_config", "config":
		writeJSON(w, map[string]any{"status": true})
	default:
		a.Log.Printf("sabapi: unsupported mode %q", mode)
		writeJSON(w, map[string]any{"status": false, "error": "not implemented"})
	}
}

func (a *API) getConfig(w http.ResponseWriter) {
	var cats []map[string]any
	cats = append(cats, map[string]any{"name": "*", "order": 0, "pp": "3", "script": "None", "dir": "", "newzbin": "", "priority": -100})
	for i, c := range a.Cfg.Get().Categories {
		cats = append(cats, map[string]any{"name": c.Name, "order": i + 1, "pp": "", "script": "Default", "dir": config.SafeName(c.Name), "newzbin": "", "priority": -100})
	}
	writeJSON(w, map[string]any{"config": map[string]any{
		"misc":       map[string]any{"complete_dir": a.Cfg.Env.Complete(), "download_dir": a.Cfg.Env.Work(), "port": "8081", "host": "0.0.0.0", "enable_tv_sorting": false, "enable_movie_sorting": false, "enable_date_sorting": false},
		"categories": cats,
		"servers":    []any{},
		"sorters":    []any{},
	}})
}

// ------------------------------------------------------------------ add

func (a *API) category(r *http.Request) string {
	c := r.FormValue("cat")
	if c == "" || c == "*" {
		if cs := a.Cfg.Get().Categories; len(cs) > 0 {
			return cs[0].Name
		}
	}
	return c
}

func (a *API) addURL(w http.ResponseWriter, r *http.Request) {
	u := r.FormValue("name")
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		writeJSON(w, map[string]any{"status": false, "error": "only http and https links can be fetched"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	nzb, fname, err := a.Fetch(ctx, u)
	if err != nil {
		a.Log.Printf("sabapi: fetch NZB failed: %v", err)
		writeJSON(w, map[string]any{"status": false, "error": "could not fetch the NZB"})
		return
	}
	name := r.FormValue("nzbname")
	if name == "" {
		name = fname
	}
	a.submit(w, r, name, nzb)
}

func (a *API) addFile(w http.ResponseWriter, r *http.Request) {
	if r.MultipartForm == nil {
		writeJSON(w, map[string]any{"status": false, "error": "no file"})
		return
	}
	for _, key := range []string{"name", "nzbfile", "file"} {
		for _, fh := range r.MultipartForm.File[key] {
			f, err := fh.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(io.LimitReader(f, 50<<20))
			f.Close()
			if err != nil {
				continue
			}
			name := r.FormValue("nzbname")
			if name == "" {
				name = fh.Filename
			}
			a.submit(w, r, name, data)
			return
		}
	}
	writeJSON(w, map[string]any{"status": false, "error": "no file"})
}

func (a *API) submit(w http.ResponseWriter, r *http.Request, name string, nzb []byte) {
	j, err := a.Engine.SubmitNZB(pipeline.SubmitNZBRequest{Category: a.category(r), Name: name, NZB: nzb})
	if err != nil {
		a.Log.Printf("sabapi: NZB rejected: %v", err)
		writeJSON(w, map[string]any{"status": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"status": true, "nzo_ids": []string{j.ID}})
}

func fetchURL(ctx context.Context, u string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := (&http.Client{Timeout: 80 * time.Second}).Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return nil, "", err
	}
	name := ""
	if cd := resp.Header.Get("Content-Disposition"); strings.Contains(cd, "filename=") {
		name = strings.Trim(cd[strings.Index(cd, "filename=")+9:], `"; `)
	}
	if name == "" {
		if pu, err := url.Parse(u); err == nil {
			name = path.Base(pu.Path)
		}
	}
	return b, name, nil
}

// ------------------------------------------------------------------ read

func mb(n int64) string { return fmt.Sprintf("%.2f", float64(n)/(1<<20)) }

func queueStatus(j job.Job) string {
	switch j.State {
	case job.Received, job.Submitted:
		return "Queued"
	}
	return "Downloading"
}

func (a *API) usenet() (queue, hist []job.Job) {
	for _, j := range a.Store.List() {
		if j.Kind != "usenet" {
			continue
		}
		switch j.State {
		case job.Cleaned:
		case job.Ready, job.Imported, job.Failed:
			hist = append(hist, j)
		default:
			queue = append(queue, j)
		}
	}
	sort.Slice(queue, func(i, k int) bool { return queue[i].Created.Before(queue[k].Created) })
	sort.Slice(hist, func(i, k int) bool { return hist[i].StateChanged.After(hist[k].StateChanged) })
	return
}

func (a *API) queue(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("name") == "delete" {
		a.deleteIDs(w, r, false)
		return
	}
	q, _ := a.usenet()
	slots := make([]map[string]any, 0, len(q))
	var total, left int64
	for i, j := range q {
		p := job.Overall(j)
		size := j.Size
		lf := int64(float64(size) * (1 - p))
		total += size
		left += lf
		slots = append(slots, map[string]any{
			"index": i, "nzo_id": j.ID, "filename": j.Name, "cat": j.Category, "status": queueStatus(j),
			"mb": mb(size), "mbleft": mb(lf), "mbmissing": "0.0", "size": humanSize(size), "sizeleft": humanSize(lf),
			"percentage": strconv.Itoa(int(p * 100)), "timeleft": "0:00:00", "priority": "Normal", "password": "",
			"avg_age": "0d", "script": "None", "direct_unpack": nil, "labels": []string{}, "unpackopts": "3",
		})
	}
	writeJSON(w, map[string]any{"queue": map[string]any{
		"version": "4.3.3", "status": "Downloading", "paused": false, "paused_all": false, "pause_int": "0",
		"speed": "0 ", "kbpersec": "0.00", "mb": mb(total), "mbleft": mb(left), "size": humanSize(total), "sizeleft": humanSize(left),
		"noofslots": len(slots), "noofslots_total": len(slots), "slots": slots, "diskspace1": "100.00", "diskspace2": "100.00",
		"diskspacetotal1": "100.00", "diskspacetotal2": "100.00", "have_warnings": "0", "timeleft": "0:00:00",
		"limit": "0", "finish": 0, "finishaction": nil,
	}})
}

func (a *API) history(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("name") == "delete" {
		a.deleteIDs(w, r, true)
		return
	}
	_, h := a.usenet()
	slots := make([]map[string]any, 0, len(h))
	for _, j := range h {
		status, fail, storage := "Completed", "", j.CompleteDir
		if j.State == job.Failed {
			status, fail, storage = "Failed", j.Reason, ""
		}
		slots = append(slots, map[string]any{
			"nzo_id": j.ID, "name": j.Name, "nzb_name": j.Name + ".nzb", "category": j.Category, "status": status,
			"fail_message": fail, "storage": storage, "path": storage, "bytes": j.Size, "size": humanSize(j.Size),
			"completed": j.StateChanged.Unix(), "download_time": int(j.StateChanged.Sub(j.Created).Seconds()),
			"script": "None", "report": "", "pp": "D", "md5sum": "", "url_info": "", "series": "", "meta": nil,
			"has_rating": false, "action_line": "", "show_details": "True", "loaded": false, "retry": 0, "stage_log": []any{},
			"postproc_time": 0, "downloaded": j.Size, "archive": false, "duplicate_key": "", "password": "",
		})
	}
	writeJSON(w, map[string]any{"history": map[string]any{
		"noofslots": len(slots), "slots": slots, "total_size": "0 B", "month_size": "0 B", "week_size": "0 B", "day_size": "0 B",
		"version": "4.3.3", "last_history_update": time.Now().Unix(),
	}})
}

// deleteIDs handles queue delete (cancel) and history delete (the arr is done with it).
func (a *API) deleteIDs(w http.ResponseWriter, r *http.Request, fromHistory bool) {
	v := r.FormValue("value")
	if v == "" {
		writeJSON(w, map[string]any{"status": false, "error": "no value"})
		return
	}
	ok := true
	for _, id := range strings.Split(v, ",") {
		id = strings.TrimSpace(id)
		if id == "all" || id == "failed" || id == "completed" {
			continue // never bulk-clear
		}
		j, found := a.Store.Get(id)
		if !found || j.Kind != "usenet" {
			continue // already gone: deleting twice is fine
		}
		if err := a.Engine.ArrDelete(id); err != nil {
			a.Log.Printf("sabapi: delete %s: %v", id, err)
			ok = false
		}
	}
	writeJSON(w, map[string]any{"status": ok, "nzo_ids": strings.Split(v, ",")})
}

func humanSize(n int64) string {
	f := float64(n)
	for _, u := range []string{"B", "KB", "MB", "GB", "TB"} {
		if f < 1024 || u == "TB" {
			if u == "B" {
				return fmt.Sprintf("%d B", n)
			}
			return fmt.Sprintf("%.1f %s", f, u)
		}
		f /= 1024
	}
	return ""
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
