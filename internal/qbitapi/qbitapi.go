// Package qbitapi answers the qBittorrent WebUI v2 calls the arrs make, from the job table.
package qbitapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/pipeline"
	"github.com/z354prance/overmynd/internal/store"
)

type API struct {
	Store  store.Repository
	Engine *pipeline.Engine
	Cfg    *config.Config
	Log    *log.Logger
	// LogRequests logs every call, to compare against what each arr version really sends.
	LogRequests bool

	mu       sync.Mutex
	sessions map[string]time.Time
}

func New(s store.LegacyRepository, e *pipeline.Engine, c *config.Config, l *log.Logger) *API {
	return NewRepository(store.FromLegacy(s), e, c, l)
}
func NewRepository(s store.Repository, e *pipeline.Engine, c *config.Config, l *log.Logger) *API {
	return &API{Store: s, Engine: e, Cfg: c, Log: l, sessions: map[string]time.Time{}, LogRequests: c.Env.LogRequests}
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", a.login)
	mux.HandleFunc("/api/v2/auth/logout", a.authed(func(w http.ResponseWriter, r *http.Request) { ok(w) }))
	mux.HandleFunc("/api/v2/app/version", a.authed(func(w http.ResponseWriter, r *http.Request) { text(w, "v4.6.7") }))
	mux.HandleFunc("/api/v2/app/webapiVersion", a.authed(func(w http.ResponseWriter, r *http.Request) { text(w, "2.9.3") }))
	mux.HandleFunc("/api/v2/app/preferences", a.authed(a.preferences))
	mux.HandleFunc("/api/v2/torrents/add", a.authed(a.add))
	mux.HandleFunc("/api/v2/torrents/info", a.authed(a.info))
	mux.HandleFunc("/api/v2/torrents/properties", a.authed(a.properties))
	mux.HandleFunc("/api/v2/torrents/files", a.authed(a.files))
	mux.HandleFunc("/api/v2/torrents/delete", a.authed(a.delete))
	mux.HandleFunc("/api/v2/torrents/categories", a.authed(a.categories))
	mux.HandleFunc("/api/v2/torrents/createCategory", a.authed(func(w http.ResponseWriter, r *http.Request) { ok(w) }))
	mux.HandleFunc("/api/v2/torrents/editCategory", a.authed(func(w http.ResponseWriter, r *http.Request) { ok(w) }))
	// Calls that only matter to a real client are accepted and ignored.
	for _, p := range []string{"setCategory", "addTags", "removeTags", "createTags", "setShareLimits",
		"topPrio", "resume", "pause", "recheck", "setLocation", "setForcedStart", "setAutoManagement"} {
		mux.HandleFunc("/api/v2/torrents/"+p, a.authed(func(w http.ResponseWriter, r *http.Request) { ok(w) }))
	}
	mux.HandleFunc("/api/v2/torrents/tags", a.authed(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, []string{}) }))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if a.LogRequests {
			a.Log.Printf("machine: unhandled %s %s", r.Method, r.URL.RequestURI())
		}
		http.NotFound(w, r)
	})
	return a.logging(mux)
}

func (a *API) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.LogRequests {
			a.Log.Printf("machine: %s %s", r.Method, r.URL.RequestURI())
		}
		next.ServeHTTP(w, r)
	})
}

// ------------------------------------------------------------------ auth

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	s := a.Cfg.Get()
	userOK := subtle.ConstantTimeCompare([]byte(r.Form.Get("username")), []byte(s.ClientUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(r.Form.Get("password")), []byte(s.ClientPassword)) == 1
	if !(userOK && passOK) {
		time.Sleep(500 * time.Millisecond)
		text(w, "Fails.")
		return
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	sid := hex.EncodeToString(b)
	a.mu.Lock()
	a.sessions[sid] = time.Now().Add(24 * time.Hour)
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "SID", Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	text(w, "Ok.")
}

func (a *API) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("SID")
		if err == nil {
			a.mu.Lock()
			exp, found := a.sessions[c.Value]
			if found && time.Now().After(exp) {
				delete(a.sessions, c.Value)
				found = false
			}
			a.mu.Unlock()
			if found {
				next(w, r)
				return
			}
		}
		http.Error(w, "Forbidden", http.StatusForbidden)
	}
}

// ------------------------------------------------------------------ add

func (a *API) add(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if err2 := r.ParseForm(); err2 != nil {
			text(w, "Fails.")
			return
		}
	}
	category := r.FormValue("category")
	added, bad := 0, 0
	for _, u := range strings.Fields(strings.ReplaceAll(r.FormValue("urls"), "|", "\n")) {
		if _, err := a.Engine.SubmitContext(r.Context(), pipeline.SubmitRequest{Category: category, Magnet: u}); err != nil {
			a.Log.Printf("machine: add url rejected: %v", err)
			bad++
		} else {
			added++
		}
	}
	if r.MultipartForm != nil {
		for _, fh := range r.MultipartForm.File["torrents"] {
			f, err := fh.Open()
			if err != nil {
				bad++
				continue
			}
			data, err := io.ReadAll(io.LimitReader(f, 16<<20))
			f.Close()
			if err == nil {
				_, err = a.Engine.SubmitContext(r.Context(), pipeline.SubmitRequest{Category: category, Torrent: data})
			}
			if err != nil {
				a.Log.Printf("machine: add file rejected: %v", err)
				bad++
			} else {
				added++
			}
		}
	}
	if added == 0 || bad > 0 {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		text(w, "Fails.")
		return
	}
	ok(w)
}

// ------------------------------------------------------------------ read

func qbitState(j job.Job) string {
	switch j.State {
	case job.Ready, job.Imported:
		return "pausedUP" // finished and not seeding
	case job.Failed:
		return "error"
	case job.Received, job.Submitted:
		return "queuedDL"
	}
	return "downloading"
}

func contentPath(j job.Job) string {
	if j.CompleteDir != "" {
		return j.CompleteDir
	}
	return j.StagingDir
}

func (a *API) torrentObject(j job.Job) map[string]any {
	p := job.Overall(j)
	done := j.State == job.Ready || j.State == job.Imported
	size := j.Size
	left := int64(float64(size) * (1 - p))
	eta := int64(8640000)
	if done {
		eta, left = 0, 0
	}
	completion := int64(-1)
	if done {
		completion = j.StateChanged.Unix()
	}
	return map[string]any{
		"hash": j.ID, "infohash_v1": j.ID, "infohash_v2": "", "name": j.Name, "size": size, "total_size": size,
		"progress": p, "dlspeed": 0, "upspeed": 0, "eta": eta, "state": qbitState(j), "category": j.Category,
		"tags": "", "save_path": saveDir(j), "content_path": contentPath(j), "amount_left": left,
		"completed": size - left, "added_on": j.Created.Unix(), "completion_on": completion,
		"last_activity": j.Updated.Unix(), "num_seeds": 0, "num_leechs": 0, "num_complete": 0, "num_incomplete": 0,
		// Seeding limits are reported as already met so the arr's "remove completed" setting fires.
		"ratio": 1.0, "ratio_limit": 0, "seeding_time": 1, "seeding_time_limit": 0,
		"max_ratio": 0, "max_seeding_time": 0, "priority": 0, "auto_tmm": false, "force_start": false,
		"super_seeding": false, "seq_dl": false, "f_l_piece_first": false, "availability": 1.0,
	}
}

func saveDir(j job.Job) string {
	if p := contentPath(j); p != "" {
		return parent(p)
	}
	return ""
}

func parent(p string) string {
	if i := strings.LastIndex(p, "/"); i > 0 {
		return p[:i]
	}
	return p
}

func (a *API) visible(j job.Job) bool {
	if j.State == job.Cleaned {
		return false
	}
	return !(j.State == job.Failed && j.ArrDeleted)
}

func (a *API) info(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cat, hasCat := q["category"]
	var hashes map[string]bool
	if h := q.Get("hashes"); h != "" && h != "all" {
		hashes = map[string]bool{}
		for _, x := range strings.Split(h, "|") {
			hashes[strings.ToLower(x)] = true
		}
	}
	out := []map[string]any{}
	jobs, err := a.Store.List(r.Context())
	if err != nil {
		http.Error(w, "job storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	for _, j := range jobs {
		if !a.visible(j) {
			continue
		}
		if hasCat && !strings.EqualFold(cat[0], j.Category) {
			continue
		}
		if hashes != nil && !hashes[j.ID] {
			continue
		}
		out = append(out, a.torrentObject(j))
	}
	writeJSON(w, out)
}

func (a *API) lookup(w http.ResponseWriter, r *http.Request, key string) (job.Job, bool) {
	j, err := a.Store.Get(r.Context(), strings.ToLower(r.FormValue(key)))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return job.Job{}, false
	}
	if err != nil {
		http.Error(w, "job storage is unavailable", http.StatusServiceUnavailable)
		return job.Job{}, false
	}
	if !a.visible(j) {
		http.NotFound(w, r)
		return job.Job{}, false
	}
	return j, true
}

func (a *API) properties(w http.ResponseWriter, r *http.Request) {
	j, found := a.lookup(w, r, "hash")
	if !found {
		return
	}
	writeJSON(w, map[string]any{
		"save_path": saveDir(j), "creation_date": j.Created.Unix(), "total_size": j.Size,
		"addition_date": j.Created.Unix(), "completion_date": j.StateChanged.Unix(),
		"share_ratio": 1.0, "seeding_time": 1, "time_elapsed": 1, "nb_connections": 0, "comment": "",
	})
}

func (a *API) files(w http.ResponseWriter, r *http.Request) {
	j, found := a.lookup(w, r, "hash")
	if !found {
		return
	}
	out := []map[string]any{}
	for i, f := range j.Files {
		out = append(out, map[string]any{"index": i, "name": f, "size": 0, "progress": job.Overall(j),
			"priority": 1, "is_seed": false, "piece_range": []int{0, 0}, "availability": 1.0})
	}
	writeJSON(w, out)
}

func (a *API) delete(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	for _, h := range strings.Split(r.FormValue("hashes"), "|") {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if err := a.Engine.ArrDelete(h); err != nil && !errors.Is(err, store.ErrNotFound) {
			if a.LogRequests {
				a.Log.Printf("machine: delete %s: %v", h, err)
			}
			http.Error(w, "could not update stored job", http.StatusServiceUnavailable)
			return
		}
	}
	ok(w)
}

func (a *API) categories(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{}
	for _, c := range a.Cfg.Get().Categories {
		out[c.Name] = map[string]any{"name": c.Name, "savePath": a.Cfg.Env.CompleteRoot(c.Name)}
	}
	writeJSON(w, out)
}

func (a *API) preferences(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"save_path": a.Cfg.Env.Complete(), "temp_path_enabled": false, "auto_tmm_enabled": false,
		"max_ratio_enabled": true, "max_ratio": 0.0, "max_ratio_act": 0,
		"max_seeding_time_enabled": false, "max_seeding_time": -1,
		"queueing_enabled": false, "dht": false,
	})
}

// -------------------------------------------------------------- helpers

func text(w http.ResponseWriter, s string) {
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	_, _ = io.WriteString(w, s)
}

func ok(w http.ResponseWriter) { text(w, "Ok.") }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
