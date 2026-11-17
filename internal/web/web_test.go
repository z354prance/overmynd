package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/z354prance/overmynd/internal/storage"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/z354prance/overmynd/internal/auth"
	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/pipeline"
	"github.com/z354prance/overmynd/internal/store"
	"github.com/z354prance/overmynd/internal/web"
)

type env struct {
	t   *testing.T
	srv *httptest.Server
	st  *store.Store
	cfg *config.Config
	au  *auth.Auth
	cl  *http.Client
}

func newEnv(t *testing.T) *env {
	dir := t.TempDir()
	e := config.Env{ConfigDir: filepath.Join(dir, "config"), DataDir: filepath.Join(dir, "data")}
	cfg, err := config.Load(e)
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Get()
	s.Categories = []config.Category{{Name: "radarr", Kind: "radarr", ArrURL: "http://radarr", ArrAPIKey: "TOP-SECRET-ARR-KEY"}}
	s.QBit.Password = "TOP-SECRET-QBIT-PW"
	_ = cfg.Set(s)
	st, _ := store.Open(filepath.Join(e.ConfigDir, "jobs.json"))
	au, err := auth.New(e.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	l := log.New(io.Discard, "", 0)
	srv := httptest.NewServer(web.New(st, pipeline.New(st, cfg, l), cfg, au, l).Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &env{t, srv, st, cfg, au, &http.Client{Jar: jar}}
}

func (e *env) do(method, path string, body any, header bool) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	if header {
		req.Header.Set("X-Undermynd-Request", "1")
	}
	resp, err := e.cl.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func (e *env) signIn() {
	code, b := e.do("POST", "/admin/api/setup", map[string]string{"key": e.au.SetupKey(), "password": "a long enough password"}, true)
	if code != 200 {
		e.t.Fatalf("setup: %d %s", code, b)
	}
	var res struct{ TotpSecret string }
	_ = json.Unmarshal(b, &res)
	if code, b = e.do("POST", "/admin/api/setup/confirm", map[string]string{"password": "a long enough password", "code": auth.CodeAt(res.TotpSecret, time.Now())}, true); code != 200 {
		e.t.Fatalf("confirm: %d %s", code, b)
	}
	if code, b = e.do("POST", "/admin/api/login", map[string]string{"password": "a long enough password", "code": auth.CodeAt(res.TotpSecret, time.Now().Add(30*time.Second))}, true); code != 200 {
		e.t.Fatalf("login: %d %s", code, b)
	}
}

func TestPublicViewLeaksNothing(t *testing.T) {
	e := newEnv(t)
	_ = e.st.Put(job.Job{ID: "abcdef0123456789abcdef0123456789abcdef01", Name: "Public.Title.2024", Category: "radarr", Source: "radarr",
		State: job.Failed, Reason: "internal path /data/undermynd/staging/x failed", SavePath: "/data/secret/path", Backend: "qbittorrent",
		Magnet: "magnet:?xt=urn:btih:abc&tr=http://tracker/announce?passkey=SEKRET", Created: time.Now()})
	code, b := e.do("GET", "/api/public/jobs", nil, false)
	if code != 200 {
		t.Fatal(code)
	}
	body := string(b)
	for _, leak := range []string{"/data", "passkey", "SEKRET", "qbittorrent", "abcdef0123", "internal path"} {
		if strings.Contains(body, leak) {
			t.Fatalf("public view leaked %q: %s", leak, body)
		}
	}
	if !strings.Contains(body, "Public.Title.2024") {
		t.Fatal("title should be public")
	}
}

func TestAdminRoutesNeedSignIn(t *testing.T) {
	e := newEnv(t)
	for _, c := range [][2]string{{"GET", "/admin/api/jobs"}, {"GET", "/admin/api/settings"}, {"GET", "/admin/api/jobs/x"}} {
		if code, _ := e.do(c[0], c[1], nil, true); code != http.StatusUnauthorized {
			t.Fatalf("%s %s without a session: %d", c[0], c[1], code)
		}
	}
	for _, c := range [][2]string{{"POST", "/admin/api/jobs/x/cancel"}, {"PUT", "/admin/api/settings"}, {"POST", "/admin/api/settings/client-password"}} {
		if code, _ := e.do(c[0], c[1], map[string]any{}, true); code != http.StatusUnauthorized {
			t.Fatalf("%s %s without a session: %d", c[0], c[1], code)
		}
	}
}

func TestStateChangesNeedHeaderAndSameOrigin(t *testing.T) {
	e := newEnv(t)
	e.signIn()
	if code, _ := e.do("POST", "/admin/api/jobs/x/cancel", nil, false); code != http.StatusForbidden {
		t.Fatalf("missing header must be refused, got %d", code)
	}
	req, _ := http.NewRequest("POST", e.srv.URL+"/admin/api/jobs/x/cancel", nil)
	req.Header.Set("X-Undermynd-Request", "1")
	req.Header.Set("Origin", "https://evil.example")
	resp, _ := e.cl.Do(req)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin must be refused, got %d", resp.StatusCode)
	}
}

func TestSettingsNeverReturnSecrets(t *testing.T) {
	e := newEnv(t)
	e.signIn()
	code, b := e.do("GET", "/admin/api/settings", nil, false)
	if code != 200 {
		t.Fatal(code)
	}
	if strings.Contains(string(b), "TOP-SECRET") || strings.Contains(string(b), string(e.cfg.Get().ClientPassword)) {
		t.Fatalf("settings leaked a secret: %s", b)
	}
	if !strings.Contains(string(b), `"hasArrApiKey":true`) || !strings.Contains(string(b), `"hasPassword":true`) {
		t.Fatalf("should say whether a secret is set: %s", b)
	}
	// saving with blank secret fields keeps the stored values
	var v map[string]any
	_ = json.Unmarshal(b, &v)
	if code, _ = e.do("PUT", "/admin/api/settings", v, true); code != 200 {
		t.Fatal(code)
	}
	got := e.cfg.Get()
	if string(got.Categories[0].ArrAPIKey) != "TOP-SECRET-ARR-KEY" || string(got.QBit.Password) != "TOP-SECRET-QBIT-PW" {
		t.Fatal("blank secret fields must keep the stored secret")
	}
}

func TestSecretsEncryptedOnDisk(t *testing.T) {
	e := newEnv(t)
	raw, err := os.ReadFile(filepath.Join(e.cfg.Env.ConfigDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "TOP-SECRET") {
		t.Fatalf("a secret is stored in plain text: %s", raw)
	}
}

func TestSetupNeedsKeyAndWorksOnce(t *testing.T) {
	e := newEnv(t)
	if code, _ := e.do("POST", "/admin/api/setup", map[string]string{"key": "guess", "password": "a long enough password"}, true); code != http.StatusForbidden {
		t.Fatalf("setup without the key must be refused, got %d", code)
	}
	e.signIn()
	if code, _ := e.do("POST", "/admin/api/setup", map[string]string{"key": "anything", "password": "another long password"}, true); code != http.StatusBadRequest && code != http.StatusForbidden {
		t.Fatalf("a second setup must fail, got %d", code)
	}
}

func TestLoginFailureDoesNotSayWhich(t *testing.T) {
	e := newEnv(t)
	e.signIn()
	_, b := e.do("POST", "/admin/api/login", map[string]string{"password": "wrong password!!", "code": "000000"}, true)
	if strings.Contains(strings.ToLower(string(b)), "password") && !strings.Contains(string(b), "sign-in failed") {
		t.Fatalf("login error reveals too much: %s", b)
	}
}

func TestPagesServe(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/", "/admin", "/assets/app.css", "/assets/admin.js", "/dash/js/app.js", "/dash/css/app.css", "/dash/css/terminal.css", "/dash/css/redesign.css", "/dash/css/wire.css", "/assets/favicon.svg", "/assets/logo.svg", "/health", "/api/v1/activity", "/api/v1/recently-added", "/api/v1/playback", "/api/v1/auth/status", "/api/v1/media-request/status"} {
		if code, _ := e.do("GET", p, nil, false); code != 200 {
			t.Fatalf("%s: %d", p, code)
		}
	}
}

func TestLiveStreamAndNewAdminRoutesNeedSignIn(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/admin/api/events", "/admin/api/system"} {
		if code, _ := e.do("GET", p, nil, false); code != 401 {
			t.Fatalf("%s must need a session, got %d", p, code)
		}
	}
	if code, _ := e.do("POST", "/admin/api/intake", map[string]bool{"paused": true}, true); code != 401 {
		t.Fatalf("intake must need a session, got %d", code)
	}
	if code, _ := e.do("POST", "/admin/api/tdarr/test", map[string]string{"url": "http://x"}, true); code != 401 {
		t.Fatalf("tdarr test must need a session, got %d", code)
	}
	e.signIn()
	_ = e.st.Put(job.Job{ID: "abc", Name: "Stream.Me", Source: "radarr", Category: "radarr", State: job.Downloading, Progress: 0.5,
		Created: time.Now(), StateChanged: time.Now()})
	req, _ := http.NewRequest("GET", e.srv.URL+"/admin/api/events", nil)
	resp, err := e.cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected an event stream, got %q", ct)
	}
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	got := string(buf[:n])
	if !strings.Contains(got, "event: jobs") || !strings.Contains(got, "Stream.Me") {
		t.Fatalf("the first event should carry the job list, got %q", got)
	}
	if strings.Contains(got, "TOP-SECRET") {
		t.Fatal("the stream must never carry secrets")
	}
}

func TestPauseIntakeAndSystem(t *testing.T) {
	e := newEnv(t)
	e.signIn()
	if code, b := e.do("POST", "/admin/api/intake", map[string]bool{"paused": true}, true); code != 200 {
		t.Fatalf("pause: %d %s", code, b)
	}
	if !e.cfg.Get().IntakePaused {
		t.Fatal("pause must be saved")
	}
	code, b := e.do("GET", "/admin/api/system", nil, false)
	if code != 200 || !strings.Contains(string(b), `"paused":true`) {
		t.Fatalf("system should report paused: %d %s", code, b)
	}
}

// Renaming a category in the form must not drop the arr key that was saved under the old name.
func TestRenamingACategoryKeepsItsArrKey(t *testing.T) {
	e := newEnv(t)
	e.signIn()
	_, b := e.do("GET", "/admin/api/settings", nil, false)
	var v map[string]any
	_ = json.Unmarshal(b, &v)
	cats := v["categories"].([]any)
	c := cats[0].(map[string]any)
	c["origName"] = c["name"]
	c["name"] = "renamed"
	if code, _ := e.do("PUT", "/admin/api/settings", v, true); code != 200 {
		t.Fatal(code)
	}
	got := e.cfg.Get()
	if got.Categories[0].Name != "renamed" || string(got.Categories[0].ArrAPIKey) != "TOP-SECRET-ARR-KEY" {
		t.Fatalf("a renamed category must keep its key: %+v", got.Categories[0])
	}
}

// The dashboard is Overmynd's page; it needs each job as a lifecycle that points at the
// download or processing record it is currently in.
func TestDashboardActivityShape(t *testing.T) {
	e := newEnv(t)
	_ = e.st.Put(job.Job{ID: "aaa", Name: "Show.S01E01", Category: "tv", Source: "sonarr", Kind: "torrent", Backend: "torbox", State: job.Downloading, Progress: 0.5, Size: 1000, Created: time.Now()})
	_ = e.st.Put(job.Job{ID: "bbb", Name: "Other.S01E02", Category: "tv", Source: "sonarr", State: job.Transcoding, TranscodeProgress: 0.25, Created: time.Now()})
	_ = e.st.Put(job.Job{ID: "ccc", Name: "Done.S01E03", Category: "tv", Source: "sonarr", State: job.Cleaned, Created: time.Now()})
	code, b := e.do("GET", "/api/v1/activity", nil, false)
	if code != 200 {
		t.Fatal(code)
	}
	var got struct {
		Lifecycles []struct {
			Title string `json:"title"`
			Kind  string `json:"kind"`
			Stage string `json:"stage"`
			Refs  []struct {
				Source     string `json:"source"`
				RecordType string `json:"record_type"`
			} `json:"references"`
		} `json:"lifecycles"`
		Downloads struct {
			Downloads []struct {
				Source   string   `json:"source"`
				Status   string   `json:"status"`
				Progress *float64 `json:"progress"`
				SizeLeft int64    `json:"size_left"`
			} `json:"downloads"`
		} `json:"download_snapshot"`
		Processing struct {
			Jobs []struct {
				Source   string  `json:"source"`
				Progress float64 `json:"progress"`
			} `json:"jobs"`
		} `json:"processing_snapshot"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err, string(b))
	}
	if len(got.Lifecycles) != 2 {
		t.Fatalf("finished jobs must not appear: %s", b)
	}
	if len(got.Downloads.Downloads) != 1 || got.Downloads.Downloads[0].Source != "torboxarr" || got.Downloads.Downloads[0].SizeLeft != 500 || *got.Downloads.Downloads[0].Progress != 50 {
		t.Fatalf("download record: %s", b)
	}
	if len(got.Processing.Jobs) != 1 || got.Processing.Jobs[0].Source != "tdarr" || got.Processing.Jobs[0].Progress != 25 {
		t.Fatalf("processing record: %s", b)
	}
	for _, l := range got.Lifecycles {
		if l.Kind != "episode" || len(l.Refs) != 1 {
			t.Fatalf("lifecycle: %+v", l)
		}
	}
}

// Overmynd was embedded in a frame on the owner's site; the public and admin pages may be,
// by the configured sites only.
func TestFrameAncestors(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest("GET", e.srv.URL+"/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'self' https://xivix.cc") || resp.Header.Get("X-Frame-Options") != "" {
		t.Fatalf("public page must be embeddable: %q", csp)
	}
	req, _ = http.NewRequest("GET", e.srv.URL+"/admin", nil)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'self' https://xivix.cc") || resp.Header.Get("X-Frame-Options") != "" {
		t.Fatalf("the admin page may be framed by the same sites only: %q", csp)
	}
	t.Setenv("UNDERMYND_FRAME_ANCESTORS", "'none'")
	e2 := newEnv(t)
	req, _ = http.NewRequest("GET", e2.srv.URL+"/admin", nil)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("embedding must be switchable off")
	}
}

// The storage readout comes from the media share (UNDERMYND_STORAGE_ROOT), not the downloads disk.
func TestStorageEndpointReadsTheMediaRoot(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "Movies"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "Movies", "a.mkv"), make([]byte, 4096), 0o644)
	dir := t.TempDir()
	cfg, _ := config.Load(config.Env{ConfigDir: filepath.Join(dir, "c"), DataDir: filepath.Join(dir, "d")})
	st, _ := store.Open(filepath.Join(dir, "c", "jobs.json"))
	au, _ := auth.New(filepath.Join(dir, "c"))
	l := log.New(io.Discard, "", 0)
	srv := web.New(st, pipeline.New(st, cfg, l), cfg, au, l)
	srv.Storage = storage.New(root)
	go srv.Storage.Run(context.Background(), func() time.Duration { return time.Hour })
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	get := func() map[string]any {
		resp, err := http.Get(ts.URL + "/api/v1/storage")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&m)
		return m
	}
	var m map[string]any
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if m = get(); m["scanning"] == false && m["total_bytes"] != nil {
			break
		}
	}
	cats, _ := m["categories"].([]any)
	if m["enabled"] != true || len(cats) != 5 || m["total_bytes"] == nil {
		t.Fatalf("storage: %v", m)
	}
	if first := cats[0].(map[string]any); first["name"] != "Movies" || first["bytes"] != float64(4096) {
		t.Fatalf("movies: %v", cats[0])
	}
	if off := web.New(st, pipeline.New(st, cfg, l), cfg, au, l); off != nil {
		ts2 := httptest.NewServer(off.Handler())
		defer ts2.Close()
		resp, _ := http.Get(ts2.URL + "/api/v1/storage")
		var d map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&d)
		resp.Body.Close()
		if d["enabled"] != false {
			t.Fatalf("with no root set the readout must be off: %v", d)
		}
	}
}

// Now Playing hides a whole library, withholds streams it cannot place, and never leaks keys;

func TestPlaybackFromTracearr(t *testing.T) {
	tr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer TRKEY" && !strings.HasPrefix(r.URL.Path, "/api/v1/images/") {
			http.Error(w, "no", 401)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/streams"):
			io.WriteString(w, `{"data":[
{"id":"1","server_type":"emby","library_id":"9","media_title":"Hidden"},
{"id":"2","server_type":"emby","library_id":"3","media_title":"Shown","rating_key":"55","poster_url":"/api/v1/images/proxy?server=s&url=%2FItems%2F55%2FImages%2FPrimary"},
{"id":"3","server_type":"emby","media_title":"Unplaceable"}]}`)
		case strings.HasSuffix(r.URL.Path, "/recently-added"):
			io.WriteString(w, `{"data":[{"id":"a","media_id":"m1","server_id":"s","server_type":"emby","rating_key":"7","title":"Film","year":2020,"added_at":"2026-01-02T00:00:00Z"}],"meta":{}}`)
		case r.URL.Path == "/api/v1/images/proxy":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("\x89PNG\r\n\x1a\n0000000000000000"))
		default:
			io.WriteString(w, `{}`)
		}
	}))
	defer tr.Close()
	e := newEnv(t)
	s := e.cfg.Get()
	s.Playback.TracearrURL, s.Playback.TracearrKey, s.Playback.HiddenLibraries = tr.URL, "TRKEY", "9"
	_ = e.cfg.Set(s)

	_, b := e.do("GET", "/api/v1/playback", nil, false)
	if strings.Contains(string(b), "Hidden") || strings.Contains(string(b), "Unplaceable") || !strings.Contains(string(b), "Shown") || strings.Contains(string(b), "TRKEY") {
		t.Fatalf("playback filter wrong: %s", b)
	}
	var pb struct {
		Sessions []struct {
			PosterURL string `json:"poster_url"`
		} `json:"sessions"`
	}
	_ = json.Unmarshal(b, &pb)
	if len(pb.Sessions) != 1 || !strings.HasPrefix(pb.Sessions[0].PosterURL, "/api/v1/playback/poster?") {
		t.Fatalf("poster not signed: %s", b)
	}
	if code, _ := e.do("GET", pb.Sessions[0].PosterURL, nil, false); code != 200 {
		t.Fatalf("signed poster = %d", code)
	}
	if code, _ := e.do("GET", "/api/v1/playback/poster?path=%2Fapi%2Fv1%2Fimages%2Fproxy&signature=bad", nil, false); code != 404 {
		t.Fatalf("unsigned poster = %d", code)
	}
	_, b = e.do("GET", "/api/v1/recently-added", nil, false)
	if !strings.Contains(string(b), `"Film"`) || !strings.Contains(string(b), "playback/poster") {
		t.Fatalf("recently added: %s", b)
	}
}

// Donation links are public, HTTPS-only, and hide the button when empty.
func TestDonationsSettings(t *testing.T) {
	e := newEnv(t)
	s := e.cfg.Get()
	s.Donations.PayPalURL = "https://paypal.me/x"
	s.Donations.Title = "Thanks"
	_ = e.cfg.Set(s)
	_, b := e.do("GET", "/api/v1/donations", nil, false)
	if !strings.Contains(string(b), "paypal.me/x") || !strings.Contains(string(b), "Thanks") {
		t.Fatalf("donations: %s", b)
	}
	d := config.Donations{PayPalURL: "http://insecure.example"}
	if err := web.ValidDonationsForTest(&d); err == nil {
		t.Fatal("http link must be refused")
	}
	d = config.Donations{CustomURL: "https://ko-fi.com/x"}
	if err := web.ValidDonationsForTest(&d); err == nil {
		t.Fatal("a custom link needs a label")
	}
}

// Visitors search and request through Seerr as one fixed user, rate-limited, failing closed;
// decisions need the admin session.
func TestSeerrRequestBox(t *testing.T) {
	var gotUser, gotKey string
	var posted []string
	sr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotKey = r.Header.Get("X-API-User"), r.Header.Get("X-Api-Key")
		switch {
		case r.URL.Path == "/api/v1/auth/me":
			io.WriteString(w, `{"id":7}`)
		case r.URL.Path == "/api/v1/discover/movies":
			io.WriteString(w, `{"results":[{"id":11,"title":"Popular Film","posterPath":"/p.jpg"}]}`)
		case r.URL.Path == "/api/v1/search":
			io.WriteString(w, `{"results":[{"id":1,"mediaType":"movie","title":"Film","posterPath":"/../x"},{"id":2,"mediaType":"person"}]}`)
		case r.Method == "POST" && r.URL.Path == "/api/v1/request":
			posted = append(posted, "request")
			io.WriteString(w, `{"id":99,"status":1}`)
		case r.URL.Path == "/api/v1/request":
			io.WriteString(w, `{"results":[{"id":5,"status":1,"media":{"mediaType":"movie","tmdbId":3},"requestedBy":{"displayName":"Sam"}}]}`)
		case r.URL.Path == "/api/v1/movie/3":
			io.WriteString(w, `{"title":"Pending Film"}`)
		case r.URL.Path == "/api/v1/request/5":
			io.WriteString(w, `{"status":1}`)
		case r.URL.Path == "/api/v1/request/5/approve":
			posted = append(posted, "approve")
			io.WriteString(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer sr.Close()
	e := newEnv(t)
	if _, b := e.do("GET", "/api/v1/media-request/status", nil, false); !strings.Contains(string(b), "false") {
		t.Fatalf("off by default: %s", b)
	}
	if code, _ := e.do("GET", "/api/v1/media-request/search?query=film", nil, false); code != 503 {
		t.Fatalf("search must fail closed: %d", code)
	}
	s := e.cfg.Get()
	s.Requests.Enabled, s.Requests.URL, s.Requests.APIKey, s.Requests.UserID = true, sr.URL, "SEERRKEY", 7
	_ = e.cfg.Set(s)
	_, b := e.do("GET", "/api/v1/media-request/search?query=film", nil, false)
	if !strings.Contains(string(b), "Film") || strings.Contains(string(b), "person") || strings.Contains(string(b), "../") || gotUser != "7" || gotKey != "SEERRKEY" {
		t.Fatalf("search: %s user=%q", b, gotUser)
	}
	if _, b := e.do("GET", "/api/v1/media-request/discover?kind=movies", nil, false); !strings.Contains(string(b), "Popular Film") || !strings.Contains(string(b), `"mediaType":"movie"`) {
		t.Fatalf("suggestions: %s", b)
	}
	if code, _ := e.do("GET", "/api/v1/media-request/discover?kind=nonsense", nil, false); code != 502 {
		t.Fatalf("unknown list = %d", code)
	}
	if code, _ := e.do("POST", "/api/v1/media-request", map[string]any{"media_id": 1, "media_type": "tv"}, false); code != 400 {
		t.Fatalf("a show needs all seasons confirmed: %d", code)
	}
	for i := 0; i < 10; i++ {
		if code, _ := e.do("POST", "/api/v1/media-request", map[string]any{"media_id": 1, "media_type": "movie"}, false); code != 201 {
			t.Fatalf("request %d = %d", i, code)
		}
	}
	if code, _ := e.do("POST", "/api/v1/media-request", map[string]any{"media_id": 1, "media_type": "movie"}, false); code != 429 {
		t.Fatalf("11th request must be limited: %d", code)
	}
	_, b = e.do("GET", "/api/v1/requests", nil, false)
	if !strings.Contains(string(b), "Pending Film") || strings.Contains(string(b), "SEERRKEY") {
		t.Fatalf("pending list: %s", b)
	}
	if code, _ := e.do("POST", "/admin/api/requests/5/approve", nil, true); code != 401 {
		t.Fatalf("approving needs the admin session: %d", code)
	}
	if code, _ := e.do("POST", "/api/v1/requests/6/5/approve", nil, true); code != 401 {
		t.Fatalf("approving from the public page needs the admin session: %d", code)
	}
	if _, b := e.do("GET", "/api/v1/auth/status", nil, false); !strings.Contains(string(b), `"admin":false`) {
		t.Fatalf("visitors are not admin: %s", b)
	}
}

// The admin session reaches the public page, so approve/deny buttons can show there.
func TestAdminSessionIsSeenOnThePublicPage(t *testing.T) {
	e := newEnv(t)
	e.signIn()
	_, b := e.do("GET", "/api/v1/auth/status", nil, false)
	if !strings.Contains(string(b), `"admin":true`) {
		t.Fatalf("signed-in admin not recognised on the public API: %s", b)
	}
}

func TestAccessRoutesAreGuarded(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/admin/api/access", "/admin/api/access/requests"} {
		if code, _ := e.do("GET", p, nil, false); code != 401 {
			t.Fatalf("GET %s = %d, want 401", p, code)
		}
	}
	code, b := e.do("GET", "/api/v1/access/status", nil, false)
	if code != 200 || !strings.Contains(string(b), `"enabled":false`) {
		t.Fatalf("status: %d %s", code, b)
	}
	if code, _ := e.do("POST", "/api/v1/access/request", map[string]string{}, false); code != 403 {
		t.Fatalf("unguarded submit = %d, want 403", code)
	}
}
