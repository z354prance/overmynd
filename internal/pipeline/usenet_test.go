package pipeline_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/pipeline"
	"github.com/z354prance/overmynd/internal/qbitapi"
	"github.com/z354prance/overmynd/internal/sabapi"
	"github.com/z354prance/overmynd/internal/secret"
	"github.com/z354prance/overmynd/internal/store"
)

type fakeTorBox struct {
	mu       sync.Mutex
	srv      *httptest.Server
	items    map[string]map[string]any
	data     map[string]string // file id -> content
	creates  int
	state    string
	busy     bool
	deleted  []string
	linkHits int
	stuck    bool // deletes are accepted but the item stays listed
}

func newFakeTorBox() *fakeTorBox {
	f := &fakeTorBox{items: map[string]map[string]any{}, data: map[string]string{}, state: "downloading"}
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer tbtoken" {
			http.Error(w, `{"success":false,"error":"BAD_TOKEN"}`, http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("/v1/api/torrents/createtorrent", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = r.ParseMultipartForm(1 << 20)
		if r.FormValue("seed") != "3" || (r.FormValue("magnet") == "" && len(r.MultipartForm.File["file"]) == 0) {
			http.Error(w, "bad create", 400)
			return
		}
		f.creates++
		id := fmt.Sprint(100 + f.creates)
		f.items[id] = map[string]any{"id": json.Number(id), "name": r.FormValue("name"), "size": 40, "progress": 0.2,
			"download_state": "downloading", "download_finished": false, "files": []any{}}
		fmt.Fprintf(w, `{"success":true,"data":{"torrent_id":%s}}`, id)
	})
	mux.HandleFunc("/v1/api/torrents/mylist", func(w http.ResponseWriter, r *http.Request) { mux2(f, w, r, auth) })
	mux.HandleFunc("/v1/api/torrents/controltorrent", func(w http.ResponseWriter, r *http.Request) { ctl(f, w, r, auth, "torrent_id") })
	mux.HandleFunc("/v1/api/torrents/requestdl", func(w http.ResponseWriter, r *http.Request) { dl(f, w, r) })
	mux.HandleFunc("/v1/api/usenet/createusenetdownload", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.busy {
			fmt.Fprint(w, `{"success":false,"error":"ACTIVE_LIMIT","detail":"no slot"}`)
			return
		}
		_ = r.ParseMultipartForm(1 << 20)
		if len(r.MultipartForm.File["file"]) == 0 {
			http.Error(w, "no file", 400)
			return
		}
		f.creates++
		id := fmt.Sprint(100 + f.creates)
		f.items[id] = map[string]any{"id": json.Number(id), "name": r.FormValue("name"), "size": 40, "progress": 0.2,
			"download_state": "downloading", "download_finished": false, "files": []any{}}
		fmt.Fprintf(w, `{"success":true,"data":{"usenetdownload_id":%s}}`, id)
	})
	mux.HandleFunc("/v1/api/usenet/mylist", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		list := []map[string]any{}
		for _, it := range f.items {
			list = append(list, it)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": list})
	})
	mux.HandleFunc("/v1/api/usenet/controlusenetdownload", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var body struct {
			ID json.Number `json:"usenet_id"`
			Op string      `json:"operation"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.stuck {
			delete(f.items, body.ID.String())
		}
		f.deleted = append(f.deleted, body.ID.String())
		fmt.Fprint(w, `{"success":true}`)
	})
	mux.HandleFunc("/v1/api/usenet/requestdl", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.linkHits++
		content := f.data[r.URL.Query().Get("file_id")]
		f.mu.Unlock()
		if r.URL.Query().Get("token") != "tbtoken" {
			http.Error(w, "no", 403)
			return
		}
		http.ServeContent(w, r, "x", time.Time{}, strings.NewReader(content))
	})
	f.srv = httptest.NewServer(mux)
	return f
}

// finish marks an item done with the given files (path -> content).
func (f *fakeTorBox) finish(id string, files map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var fl []map[string]any
	n := 0
	for p, c := range files {
		n++
		fid := fmt.Sprintf("%s%d", id, n)
		f.data[fid] = c
		fl = append(fl, map[string]any{"id": json.Number(fid), "name": p, "size": len(c)})
	}
	it := f.items[id]
	it["files"], it["progress"], it["download_finished"], it["download_state"] = fl, 1.0, true, "cached"
}

type urig struct {
	t   *testing.T
	eng *pipeline.Engine
	tb  *fakeTorBox
	arr *fakeArr
	api *httptest.Server
	st  *store.Store
	cfg *config.Config
	now time.Time
}

func newURig(t *testing.T) *urig {
	t.Helper()
	secret.SetKeyForTest(make([]byte, 32))
	dir := t.TempDir()
	env := config.Env{ConfigDir: filepath.Join(dir, "config"), DataDir: filepath.Join(dir, "data"), TickInterval: time.Second}
	cfg, err := config.Load(env)
	if err != nil {
		t.Fatal(err)
	}
	tb, ar := newFakeTorBox(), newFakeArr()
	t.Cleanup(tb.srv.Close)
	t.Cleanup(ar.srv.Close)
	s := cfg.Get()
	s.Categories = []config.Category{{Name: "tv-sonarr", Kind: "sonarr", ArrURL: ar.srv.URL, ArrAPIKey: "arrkey"}}
	s.TorBox.Token, s.TorBox.BaseURL, s.TorBox.LinkGapSeconds = "tbtoken", tb.srv.URL, 1
	s.ClientUser, s.ClientPassword = "undermynd", "clientpw"
	if err := cfg.Set(s); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(env.ConfigDir, "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	u := &urig{t: t, tb: tb, arr: ar, cfg: cfg, st: st, now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	st.Now = func() time.Time { return u.now }
	u.eng = pipeline.New(st, cfg, log.New(os.Stderr, "", 0))
	u.eng.Now = func() time.Time { return u.now }
	api := sabapi.New(st, u.eng, cfg, log.New(os.Stderr, "", 0))
	u.api = httptest.NewServer(api.Handler())
	t.Cleanup(u.api.Close)
	return u
}

func (u *urig) tick(n int) {
	for i := 0; i < n; i++ {
		u.now = u.now.Add(time.Minute)
		u.eng.Tick(context.Background())
		u.eng.Wait()
	}
}

func (u *urig) sab(mode string, extra url.Values) map[string]any {
	q := url.Values{"mode": {mode}, "apikey": {"clientpw"}, "output": {"json"}}
	for k, v := range extra {
		q[k] = v
	}
	resp, err := http.Get(u.api.URL + "/sabnzbd/api?" + q.Encode())
	if err != nil {
		u.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func (u *urig) addNZB(name string) string {
	nzb := `<?xml version="1.0"?><nzb><file subject="x"><segments><segment bytes="40" number="1">a@b</segment></segments></file></nzb>` + name
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("name", name+".nzb")
	_, _ = fw.Write([]byte(nzb))
	_ = w.Close()
	resp, err := http.Post(u.api.URL+"/sabnzbd/api?mode=addfile&cat=tv-sonarr&apikey=clientpw&output=json", w.FormDataContentType(), &buf)
	if err != nil {
		u.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Status bool     `json:"status"`
		IDs    []string `json:"nzo_ids"`
	}
	b, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(b, &out)
	if !out.Status || len(out.IDs) != 1 {
		u.t.Fatalf("addfile: %s", b)
	}
	return out.IDs[0]
}

func (u *urig) slot(list, id string) map[string]any {
	res := u.sab(list, nil)
	box, _ := res[list].(map[string]any)
	slots, _ := box["slots"].([]any)
	for _, s := range slots {
		m := s.(map[string]any)
		if m["nzo_id"] == id {
			return m
		}
	}
	return nil
}

func TestUsenetHappyPath(t *testing.T) {
	u := newURig(t)
	if u.sab("version", nil)["version"] == nil {
		t.Fatal("version should answer without a key")
	}
	if res := u.sab("queue", url.Values{"apikey": {"wrong"}}); res["status"] != false {
		t.Fatalf("a wrong key must be refused: %v", res)
	}
	id := u.addNZB("Show.S01E01.1080p")
	if again := u.addNZB("Show.S01E01.1080p"); again != id {
		t.Fatal("the same NZB must be the same job")
	}
	u.tick(2)
	if u.tb.creates != 1 {
		t.Fatalf("expected one TorBox create, got %d", u.tb.creates)
	}
	if s := u.slot("queue", id); s == nil || s["status"] != "Downloading" {
		t.Fatalf("queue should show it downloading: %v", s)
	}
	body := strings.Repeat("v", 5000)
	u.tb.finish("101", map[string]string{
		"Show.S01E01.1080p/Show.S01E01.1080p.mkv": body,
		"Show.S01E01.1080p/readme.nfo":            "x",
		"Show.S01E01.1080p/Sample/sample.mkv":     "tiny",
	})
	u.tick(3)
	j, _ := u.st.Get(id)
	if j.State != job.Ready {
		t.Fatalf("expected ready, got %s (%s)", j.State, j.Reason)
	}
	got, err := os.ReadFile(filepath.Join(j.CompleteDir, "Show.S01E01.1080p.mkv"))
	if err != nil || string(got) != body {
		t.Fatalf("file not delivered intact: %v", err)
	}
	if exists(filepath.Join(j.CompleteDir, "readme.nfo")) {
		t.Fatal("non-media must not be copied")
	}
	if len(u.tb.deleted) != 1 {
		t.Fatal("the TorBox item must be deleted once the local copy is verified")
	}
	if s := u.slot("history", id); s == nil || s["status"] != "Completed" || s["storage"] != j.CompleteDir {
		t.Fatalf("history should hand the arr the final path: %v", s)
	}
	u.arr.mu.Lock()
	u.arr.imported[strings.ToLower(id)] = true
	u.arr.mu.Unlock()
	u.tick(3)
	j, _ = u.st.Get(id)
	if j.State != job.Cleaned {
		t.Fatalf("expected cleaned, got %s (%s)", j.State, j.Reason)
	}
	if exists(j.CompleteDir) || exists(j.NZBPath) {
		t.Fatal("nothing may be left behind")
	}
}

func TestUsenetBusyWaitsThenSucceeds(t *testing.T) {
	u := newURig(t)
	u.tb.mu.Lock()
	u.tb.busy = true
	u.tb.mu.Unlock()
	id := u.addNZB("Busy.Show.S01E02")
	u.tick(3)
	j, _ := u.st.Get(id)
	if j.State != job.Received || u.tb.creates != 0 {
		t.Fatalf("a busy TorBox must leave the job waiting, got %s (%s)", j.State, j.Reason)
	}
	u.tb.mu.Lock()
	u.tb.busy = false
	u.tb.mu.Unlock()
	u.tick(5) // the limiter backs off for two minutes
	j, _ = u.st.Get(id)
	if j.State == job.Received || j.State == job.Failed {
		t.Fatalf("job should have been submitted after the wait, got %s (%s)", j.State, j.Reason)
	}
}

func TestUsenetTorBoxErrorStateFailsThenArrDeleteCleans(t *testing.T) {
	u := newURig(t)
	id := u.addNZB("Bad.Show.S01E03")
	u.tick(2)
	for i := 0; i < 12; i++ { // every attempt TorBox reports a failure: re-submits twice, then fails
		u.tb.mu.Lock()
		for _, it := range u.tb.items {
			it["download_state"] = "failed"
		}
		u.tb.mu.Unlock()
		u.tick(1)
	}
	j, _ := u.st.Get(id)
	if j.State != job.Failed {
		t.Fatalf("expected failed, got %s (%s)", j.State, j.Reason)
	}
	if s := u.slot("history", id); s == nil || s["status"] != "Failed" {
		t.Fatalf("the arr must see the failure in history: %v", s)
	}
	u.sab("history", url.Values{"name": {"delete"}, "value": {id}})
	u.tick(3)
	j, _ = u.st.Get(id)
	if j.State != job.Cleaned {
		t.Fatalf("expected cleaned after the arr removed it, got %s", j.State)
	}
}

func mux2(f *fakeTorBox, w http.ResponseWriter, r *http.Request, auth func(http.ResponseWriter, *http.Request) bool) {
	if !auth(w, r) {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	list := []map[string]any{}
	for _, it := range f.items {
		list = append(list, it)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": list})
}

func ctl(f *fakeTorBox, w http.ResponseWriter, r *http.Request, auth func(http.ResponseWriter, *http.Request) bool, field string) {
	if !auth(w, r) {
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	defer f.mu.Unlock()
	id := fmt.Sprint(body[field])
	delete(f.items, id)
	f.deleted = append(f.deleted, id)
	fmt.Fprint(w, `{"success":true}`)
}

func dl(f *fakeTorBox, w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.linkHits++
	content := f.data[r.URL.Query().Get("file_id")]
	f.mu.Unlock()
	if r.URL.Query().Get("token") != "tbtoken" || r.URL.Query().Get("torrent_id") == "" {
		http.Error(w, "no", 403)
		return
	}
	http.ServeContent(w, r, "x", time.Time{}, strings.NewReader(content))
}

func TestTorBoxTorrent(t *testing.T) {
	u := newURig(t)
	qapi := qbitapi.New(u.st, u.eng, u.cfg, log.New(os.Stderr, "", 0))
	srv := httptest.NewServer(qapi.Handler())
	defer srv.Close()
	cl := &http.Client{}
	jar, _ := cookiejar.New(nil)
	cl.Jar = jar
	if _, err := cl.PostForm(srv.URL+"/api/v2/auth/login", url.Values{"username": {"undermynd"}, "password": {"clientpw"}}); err != nil {
		t.Fatal(err)
	}
	magnet := "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=Tor.Movie.2024"
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("urls", magnet)
	_ = w.WriteField("category", "tv-sonarr")
	_ = w.Close()
	resp, err := cl.Post(srv.URL+"/api/v2/torrents/add", w.FormDataContentType(), &buf)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("add: %v %v", err, resp)
	}
	id := "0123456789abcdef0123456789abcdef01234567"
	u.tick(2)
	if j, _ := u.st.Get(id); j.Backend != "torbox" || u.tb.creates != 1 {
		t.Fatalf("expected a TorBox torrent create, got %s creates=%d (%s)", j.Backend, u.tb.creates, j.Reason)
	}
	u.tb.finish("101", map[string]string{"Tor.Movie.2024/Tor.Movie.2024.mkv": strings.Repeat("m", 2048)})
	u.tick(3)
	j, _ := u.st.Get(id)
	if j.State != job.Ready || !exists(filepath.Join(j.CompleteDir, "Tor.Movie.2024.mkv")) {
		t.Fatalf("expected ready with the file, got %s (%s)", j.State, j.Reason)
	}
	if len(u.tb.deleted) != 1 {
		t.Fatal("TorBox torrent must be deleted after the copy")
	}
}

// A client that is slow to drop a deleted item must not hold up a job whose
// files are already safe locally; cleanup finishes the job once it is gone.
func TestSlowClientDeleteDoesNotBlockTheJob(t *testing.T) {
	u := newURig(t)
	id := u.addNZB("Slow.S01E01.1080p")
	u.tick(2)
	u.tb.mu.Lock()
	u.tb.stuck = true
	u.tb.mu.Unlock()
	u.tb.finish("101", map[string]string{"Slow.S01E01.1080p/Slow.S01E01.1080p.mkv": strings.Repeat("v", 3000)})
	u.tick(3)
	j, _ := u.st.Get(id)
	if j.State != job.Ready {
		t.Fatalf("a pulled job must go on to ready while the delete is unconfirmed, got %s (%s)", j.State, j.Reason)
	}
}

// Jobs from RSS or a watch folder were not grabbed by an arr, so nothing will
// import them: the finished files stay in the complete folder and the job ends.
func TestWatchJobKeepsItsFilesAndEnds(t *testing.T) {
	u := newURig(t)
	nzb := []byte(`<?xml version="1.0"?><nzb><file subject="x"><segments><segment bytes="40" number="1">a@b</segment></segments></file></nzb>manual`)
	j, err := u.eng.SubmitNZB(pipeline.SubmitNZBRequest{Source: "watch", Category: "tv-sonarr", Name: "Manual.Thing.2024", NZB: nzb})
	if err != nil {
		t.Fatal(err)
	}
	if j.Source != "watch" {
		t.Fatalf("source should be watch, got %q", j.Source)
	}
	u.tick(2)
	u.tb.finish("101", map[string]string{"Manual.Thing.2024/Manual.Thing.2024.mkv": strings.Repeat("m", 4000)})
	u.tick(4)
	got, _ := u.st.Get(j.ID)
	if got.State != job.Cleaned {
		t.Fatalf("expected the job to end as cleaned, got %s (%s)", got.State, got.Reason)
	}
	if !exists(filepath.Join(got.CompleteDir, "Manual.Thing.2024.mkv")) {
		t.Fatal("the delivered file must be kept")
	}
	if len(u.tb.deleted) == 0 {
		t.Fatal("the TorBox item must still be removed")
	}
}

// An arr that imports by moving leaves the complete folder empty. When its
// history never shows the import, that still counts, after a short wait.
func TestFilesMovedOutOfCompleteMeansImported(t *testing.T) {
	u := newURig(t)
	id := u.addNZB("Moved.Show.S01E01")
	u.tick(2)
	u.tb.finish("101", map[string]string{"Moved.Show.S01E01/Moved.Show.S01E01.mkv": strings.Repeat("m", 4000)})
	u.tick(3)
	j, _ := u.st.Get(id)
	if j.State != job.Ready {
		t.Fatalf("expected ready, got %s (%s)", j.State, j.Reason)
	}
	u.tick(3)
	if j, _ = u.st.Get(id); j.State != job.Ready {
		t.Fatalf("with the file still in place the job must keep waiting, got %s", j.State)
	}
	if err := os.Remove(filepath.Join(j.CompleteDir, "Moved.Show.S01E01.mkv")); err != nil {
		t.Fatal(err)
	}
	u.tick(4)
	if j, _ = u.st.Get(id); j.State != job.Cleaned {
		t.Fatalf("expected cleaned after the arr moved the files, got %s (%s)", j.State, j.Reason)
	}
}

func libraryRig(t *testing.T) (*urig, string) {
	u := newURig(t)
	lib := t.TempDir()
	for _, d := range []string{"Show Name/Season 1", "Other Show/Season 1"} {
		if err := os.MkdirAll(filepath.Join(lib, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s := u.cfg.Get()
	s.Categories = append(s.Categories, config.Category{Name: "manual", Kind: "rss", LibraryDir: lib})
	if err := u.cfg.Set(s); err != nil {
		t.Fatal(err)
	}
	return u, lib
}

func (u *urig) dropRelease(t *testing.T, tbID, release, file string) job.Job {
	t.Helper()
	nzb := []byte(`<?xml version="1.0"?><nzb><file subject="x"><segments><segment bytes="40" number="1">a@b</segment></segments></file></nzb>` + release)
	j, err := u.eng.SubmitNZB(pipeline.SubmitNZBRequest{Source: "watch", Category: "manual", Name: release, NZB: nzb})
	if err != nil {
		t.Fatal(err)
	}
	u.tick(2)
	u.tb.finish(tbID, map[string]string{release + "/" + file: strings.Repeat("e", 4000)})
	u.tick(4)
	got, _ := u.st.Get(j.ID)
	return got
}

// A release that matches a show folder is renamed like the library and moved in.
func TestLibraryPlacesAMatchingEpisode(t *testing.T) {
	u, lib := libraryRig(t)
	rel := "[SubsPlease] Show Name S2 - 07 (1080p) [ABCD1234]"
	got := u.dropRelease(t, "101", rel, rel+".mkv")
	if got.State != job.Cleaned {
		t.Fatalf("expected cleaned, got %s (%s)", got.State, got.Reason)
	}
	want := filepath.Join(lib, "Show Name", "Season 2", "Show Name S2 - 7 (1080p).mkv")
	if !exists(want) {
		t.Fatalf("the episode should be placed at %s (%s)", want, got.Reason)
	}
	if exists(got.CompleteDir) {
		t.Fatal("nothing should be left in the complete folder when everything was placed")
	}
}

// A release with no clear show folder stays where it was delivered and the job ends.
func TestLibraryLeavesWhatItCannotPlace(t *testing.T) {
	u, lib := libraryRig(t)
	rel := "[SubsPlease] Brand New Show - 01 (1080p) [ABCD1234]"
	got := u.dropRelease(t, "101", rel, rel+".mkv")
	if got.State != job.Cleaned {
		t.Fatalf("the job must end, got %s (%s)", got.State, got.Reason)
	}
	if !strings.Contains(got.Reason, "left in") {
		t.Fatalf("the reason should say where the file was left: %s", got.Reason)
	}
	if !exists(filepath.Join(got.CompleteDir, rel+".mkv")) {
		t.Fatal("the unplaced file must stay in the complete folder, untouched")
	}
	if exists(filepath.Join(lib, "Brand New Show")) {
		t.Fatal("no folder may be created on a guess")
	}
}

// An episode that is already in the library is never overwritten.
func TestLibraryNeverOverwrites(t *testing.T) {
	u, lib := libraryRig(t)
	dst := filepath.Join(lib, "Show Name", "Season 1", "Show Name - 3 (1080p).mkv")
	if err := os.WriteFile(dst, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel := "[SubsPlease] Show Name - 03 (1080p) [ABCD1234]"
	got := u.dropRelease(t, "101", rel, rel+".mkv")
	if b, _ := os.ReadFile(dst); string(b) != "original" {
		t.Fatal("an existing library file must not be replaced")
	}
	if !strings.Contains(got.Reason, "already exists") || !exists(filepath.Join(got.CompleteDir, rel+".mkv")) {
		t.Fatalf("the new file should stay put with a reason: %s", got.Reason)
	}
}

// Files delivered before a library folder was set can be placed afterwards.
func TestSweepPlacesOlderFiles(t *testing.T) {
	u, lib := libraryRig(t)
	root := u.cfg.Env.CompleteRoot("manual")
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	put := func(dir, name string) string {
		p := filepath.Join(root, dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, old, old)
		return p
	}
	put("a", "[SubsPlease] Show Name - 03 (1080p) [AAAA1111].mkv")
	stay := put("b", "[SubsPlease] Unknown Show - 01 (1080p) [BBBB2222].mkv")
	msg, err := u.eng.SweepLibrary("manual")
	if err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(lib, "Show Name", "Season 1", "Show Name - 3 (1080p).mkv")) {
		t.Fatalf("matching file not placed: %s", msg)
	}
	if !exists(stay) {
		t.Fatal("unmatched file must stay")
	}
	if exists(filepath.Join(root, "a")) {
		t.Fatal("emptied folder should be removed")
	}
}

type failArr struct{ told []string }

func (f *failArr) Imported(context.Context, string) (bool, error) { return false, nil }
func (f *failArr) FailDownload(_ context.Context, id string) (bool, error) {
	f.told = append(f.told, id)
	return true, nil
}

// A TorBox failure must reach the arr (blocklist and search again) and the job
// must end by itself, even if the arr never removes the download.
func TestTorBoxFailureTellsTheArrAndEndsTheJob(t *testing.T) {
	u := newURig(t)
	fa := &failArr{}
	u.eng.ArrFor = func(config.Category) pipeline.ImportChecker { return fa }
	id := u.addNZB("Bad.Show.S01E04")
	u.tick(2)
	for i := 0; i < 14; i++ {
		u.tb.mu.Lock()
		for _, it := range u.tb.items {
			it["download_state"] = "failed"
		}
		u.tb.mu.Unlock()
		u.tick(1)
	}
	j, _ := u.st.Get(id)
	if j.State != job.Cleaned {
		t.Fatalf("expected the failed job to end, got %s (%s)", j.State, j.Reason)
	}
	if len(fa.told) != 1 {
		t.Fatalf("the arr must be told exactly once, was told %d times", len(fa.told))
	}
}
