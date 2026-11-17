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

	"github.com/z354prance/overmynd/internal/bencode"
	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/pipeline"
	"github.com/z354prance/overmynd/internal/qbitapi"
	"github.com/z354prance/overmynd/internal/secret"
	"github.com/z354prance/overmynd/internal/store"
)

// fakeQBit is a minimal real-client stand-in that the engine's qBittorrent backend talks to.
type fakeQBit struct {
	mu       sync.Mutex
	items    map[string]*fqItem
	srv      *httptest.Server
	loggedIn bool
}
type fqItem struct {
	savepath string
	progress float64
	state    string
	files    []fqFile
}
type fqFile struct {
	name string
	size int64
}

func newFakeQBit() *fakeQBit {
	f := &fakeQBit{items: map[string]*fqItem{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "x", Path: "/"})
		fmt.Fprint(w, "Ok.")
	})
	mux.HandleFunc("/api/v2/torrents/add", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		var hash string
		if fh := r.MultipartForm.File["torrents"]; len(fh) > 0 {
			f2, _ := fh[0].Open()
			b, _ := io.ReadAll(f2)
			hash, _, _, _ = bencode.Torrent(b)
		} else {
			hash, _, _ = bencode.Magnet(r.FormValue("urls"))
		}
		f.mu.Lock()
		f.items[hash] = &fqItem{savepath: r.FormValue("savepath"), state: "downloading"}
		f.mu.Unlock()
		fmt.Fprint(w, "Ok.")
	})
	mux.HandleFunc("/api/v2/torrents/info", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		out := []map[string]any{}
		for h, it := range f.items {
			if want := r.URL.Query().Get("hashes"); want != "" && want != h {
				continue
			}
			out = append(out, map[string]any{"hash": h, "state": it.state, "progress": it.progress})
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/api/v2/torrents/files", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		out := []map[string]any{}
		if it := f.items[r.URL.Query().Get("hash")]; it != nil {
			for _, x := range it.files {
				out = append(out, map[string]any{"name": x.name, "size": x.size})
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/api/v2/torrents/delete", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		it := f.items[r.FormValue("hashes")]
		delete(f.items, r.FormValue("hashes"))
		f.mu.Unlock()
		if it != nil && r.FormValue("deleteFiles") == "true" {
			// a real client deletes the files it still has; moved files are already gone
			for _, x := range it.files {
				_ = os.Remove(filepath.Join(it.savepath, x.name))
			}
		}
	})
	f.srv = httptest.NewServer(mux)
	return f
}

// finish writes real files to the job's save path and marks the item complete.
func (f *fakeQBit) finish(hash string, files map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	it := f.items[hash]
	it.progress, it.state = 1, "stalledUP"
	for name, content := range files {
		p := filepath.Join(it.savepath, name)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(content), 0o644)
		it.files = append(it.files, fqFile{name: name, size: int64(len(content))})
	}
}

func (f *fakeQBit) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.items) }

type fakeArr struct {
	mu       sync.Mutex
	imported map[string]bool
	srv      *httptest.Server
}

func newFakeArr() *fakeArr {
	a := &fakeArr{imported: map[string]bool{}}
	a.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "arrkey" || !strings.HasPrefix(r.URL.Path, "/api/v3/history") {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		id := r.URL.Query().Get("downloadId")
		a.mu.Lock()
		defer a.mu.Unlock()
		recs := []map[string]any{{"eventType": "grabbed", "downloadId": id}}
		if a.imported[strings.ToLower(id)] {
			recs = append(recs, map[string]any{"eventType": "downloadFolderImported", "downloadId": id})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"records": recs})
	}))
	return a
}

type rig struct {
	t      *testing.T
	eng    *pipeline.Engine
	api    *httptest.Server
	cl     *http.Client
	qb     *fakeQBit
	arr    *fakeArr
	cfg    *config.Config
	now    time.Time
	st     *store.Store
	dataDi string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	dir := t.TempDir()
	env := config.Env{ConfigDir: filepath.Join(dir, "config"), DataDir: filepath.Join(dir, "data"), TickInterval: time.Second}
	cfg, err := config.Load(env)
	if err != nil {
		t.Fatal(err)
	}
	qb, ar := newFakeQBit(), newFakeArr()
	t.Cleanup(qb.srv.Close)
	t.Cleanup(ar.srv.Close)
	s := cfg.Get()
	s.Categories = []config.Category{{Name: "radarr", Kind: "radarr", ArrURL: ar.srv.URL, ArrAPIKey: "arrkey"}}
	s.QBit = config.QBit{URL: qb.srv.URL, Username: "u", Password: "p", SaveDir: filepath.Join(env.DataDir, "undermynd", "client")}
	s.ClientUser, s.ClientPassword = "undermynd", "clientpw"
	if err := cfg.Set(s); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(env.ConfigDir, "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{t: t, qb: qb, arr: ar, cfg: cfg, st: st, now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	st.Now = func() time.Time { return r.now }
	r.eng = pipeline.New(st, cfg, log.New(os.Stderr, "", 0))
	r.eng.Now = func() time.Time { return r.now }
	api := qbitapi.New(st, r.eng, cfg, log.New(os.Stderr, "", 0))
	r.api = httptest.NewServer(api.Handler())
	t.Cleanup(r.api.Close)
	jar, _ := cookiejar.New(nil)
	r.cl = &http.Client{Jar: jar}
	resp, err := r.cl.PostForm(r.api.URL+"/api/v2/auth/login", url.Values{"username": {"undermynd"}, "password": {"clientpw"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "Ok." {
		t.Fatalf("login: %q", b)
	}
	return r
}

func (r *rig) tick(n int) {
	for i := 0; i < n; i++ {
		r.now = r.now.Add(time.Minute)
		r.eng.Tick(context.Background())
	}
}

func torrentBytes(name string, length int) []byte {
	info := fmt.Sprintf("d6:lengthi%de4:name%d:%s12:piece lengthi16384e6:pieces20:xxxxxxxxxxxxxxxxxxxxe", length, len(name), name)
	return []byte("d4:info" + info + "e")
}

func (r *rig) addTorrent(name, category string, length int) string {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("category", category)
	fw, _ := w.CreateFormFile("torrents", "x.torrent")
	fw.Write(torrentBytes(name, length))
	_ = w.Close()
	resp, err := r.cl.Post(r.api.URL+"/api/v2/torrents/add", w.FormDataContentType(), &buf)
	if err != nil {
		r.t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(b) != "Ok." {
		r.t.Fatalf("add: %d %q", resp.StatusCode, b)
	}
	id, _, _, _ := bencode.Torrent(torrentBytes(name, length))
	return id
}

func (r *rig) info(id string) map[string]any {
	resp, err := r.cl.Get(r.api.URL + "/api/v2/torrents/info?hashes=" + id)
	if err != nil {
		r.t.Fatal(err)
	}
	var list []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

func (r *rig) state(id string) job.State {
	j, _ := r.st.Get(id)
	return j.State
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestHappyPath(t *testing.T) {
	r := newRig(t)
	secret.SetKeyForTest(make([]byte, 32))
	id := r.addTorrent("My.Movie.2024.1080p", "radarr", 1000)

	r.tick(2) // received -> submitted -> downloading
	if r.qb.count() != 1 {
		t.Fatalf("expected the client to hold the item, has %d", r.qb.count())
	}
	if got := r.info(id); got == nil || got["state"] != "queuedDL" && got["state"] != "downloading" {
		t.Fatalf("while downloading the arr should see a downloading item: %v", got)
	}

	files := map[string]string{
		"My.Movie.2024.1080p/My.Movie.2024.1080p.mkv": strings.Repeat("v", 1000),
		"My.Movie.2024.1080p/info.nfo":                "text",
		"My.Movie.2024.1080p/poster.jpg":              "img",
		"My.Movie.2024.1080p/Sample/sample.mkv":       "tiny",
		"My.Movie.2024.1080p/My.Movie.2024.1080p.srt": "subs",
	}
	r.qb.finish(id, files)
	r.tick(1) // downloaded -> staged -> ready
	if r.state(id) != job.Ready {
		j, _ := r.st.Get(id)
		t.Fatalf("expected ready, got %s (%s)", j.State, j.Reason)
	}
	j, _ := r.st.Get(id)
	if !exists(filepath.Join(j.CompleteDir, "My.Movie.2024.1080p.mkv")) || !exists(filepath.Join(j.CompleteDir, "My.Movie.2024.1080p.srt")) {
		t.Fatalf("media files missing from %s", j.CompleteDir)
	}
	for _, junk := range []string{"info.nfo", "poster.jpg", "Sample"} {
		if exists(filepath.Join(j.CompleteDir, junk)) {
			t.Fatalf("%s should never enter the pipeline", junk)
		}
	}
	if r.qb.count() != 0 {
		t.Fatal("the client item must be removed once the local copy is verified")
	}
	if exists(j.SavePath) {
		t.Fatal("client save path should be removed")
	}
	got := r.info(id)
	if got["state"] != "pausedUP" || got["progress"].(float64) != 1 || got["content_path"] != j.CompleteDir {
		t.Fatalf("arr should see completed item with the final path: %v", got)
	}

	r.tick(2)
	if r.state(id) != job.Ready {
		t.Fatalf("must wait for the arr import, got %s", r.state(id))
	}

	r.arr.mu.Lock()
	r.arr.imported[id] = true
	r.arr.mu.Unlock()
	r.tick(2) // imported -> cleaned (reconcile interval is 30s, ticks are 1 minute)
	if r.state(id) != job.Cleaned {
		j, _ := r.st.Get(id)
		t.Fatalf("expected cleaned, got %s (%s)", j.State, j.Reason)
	}
	if exists(j.CompleteDir) || exists(r.cfg.Env.StagingDir(id)) || exists(j.TorrentPath) {
		t.Fatal("nothing may be left behind")
	}
	if r.info(id) != nil {
		t.Fatal("a cleaned job must no longer be listed to the arr")
	}
}

func TestArrDeleteAfterImportCleansUp(t *testing.T) {
	r := newRig(t)
	id := r.addTorrent("Some.Movie", "radarr", 10)
	r.tick(2)
	r.qb.finish(id, map[string]string{"Some.Movie.mkv": "0123456789"})
	r.tick(1)
	if r.state(id) != job.Ready {
		t.Fatalf("not ready: %s", r.state(id))
	}
	resp, _ := r.cl.PostForm(r.api.URL+"/api/v2/torrents/delete", url.Values{"hashes": {id}, "deleteFiles": {"true"}})
	resp.Body.Close()
	r.tick(3)
	j, _ := r.st.Get(id)
	if j.State != job.Cleaned || exists(j.CompleteDir) {
		t.Fatalf("delete from the arr should finish the job: %s %s", j.State, j.CompleteDir)
	}
}

func TestFailureIsReportedThenCleaned(t *testing.T) {
	r := newRig(t)
	id := r.addTorrent("Broken.Movie", "radarr", 10)
	r.tick(2)
	for i := 0; i < 5 && r.state(id) != job.Failed; i++ {
		r.qb.mu.Lock()
		if it := r.qb.items[id]; it != nil {
			it.state = "error"
		}
		r.qb.mu.Unlock()
		r.tick(2)
	}
	if r.state(id) != job.Failed {
		t.Fatalf("expected failed, got %s", r.state(id))
	}
	if got := r.info(id); got == nil || got["state"] != "error" {
		t.Fatalf("the arr must be told about the failure: %v", got)
	}
	if r.qb.count() != 0 {
		t.Fatal("a failed job's client item must be removed")
	}
	resp, _ := r.cl.PostForm(r.api.URL+"/api/v2/torrents/delete", url.Values{"hashes": {id}, "deleteFiles": {"true"}})
	resp.Body.Close()
	r.tick(2)
	if r.state(id) != job.Cleaned || r.info(id) != nil {
		t.Fatalf("after the arr handles the failure the job is cleaned, got %s", r.state(id))
	}
}

func TestNoMediaFails(t *testing.T) {
	r := newRig(t)
	id := r.addTorrent("Only.Junk", "radarr", 10)
	r.tick(2)
	r.qb.finish(id, map[string]string{"readme.txt": "x"})
	r.tick(1)
	j, _ := r.st.Get(id)
	if j.State != job.Failed || !strings.Contains(j.Reason, "no media") {
		t.Fatalf("got %s %q", j.State, j.Reason)
	}
}

func TestRestartResumes(t *testing.T) {
	r := newRig(t)
	id := r.addTorrent("Resume.Me", "radarr", 10)
	r.tick(2)
	// a new engine over the same persisted store continues from the stored state
	st2, err := store.Open(filepath.Join(r.cfg.Env.ConfigDir, "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	st2.Now = func() time.Time { return r.now }
	e2 := pipeline.New(st2, r.cfg, nil)
	e2.Now = func() time.Time { return r.now }
	r.qb.finish(id, map[string]string{"Resume.Me.mkv": "0123456789"})
	r.now = r.now.Add(time.Minute)
	e2.Tick(context.Background())
	if j, _ := st2.Get(id); j.State != job.Ready {
		t.Fatalf("restarted engine should carry on to ready, got %s (%s)", j.State, j.Reason)
	}
}

func TestMagnetIntake(t *testing.T) {
	h, name, err := bencode.Magnet("magnet:?xt=urn:btih:C12FE1C06BBA254A9DC9F519B335AA7C1367A88A&dn=Some.Show.S01")
	if err != nil || h != "c12fe1c06bba254a9dc9f519b335aa7c1367a88a" || name != "Some.Show.S01" {
		t.Fatalf("%v %q %q", err, h, name)
	}
}
