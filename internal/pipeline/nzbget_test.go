package pipeline_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/z354prance/undermynd/internal/job"
)

type fakeNZBGet struct {
	mu      sync.Mutex
	srv     *httptest.Server
	state   string // "queue" | "history"
	dir     string
	removed bool
	appends int
}

func newFakeNZBGet(dir string) *fakeNZBGet {
	f := &fakeNZBGet{state: "queue", dir: dir}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "ng" || p != "pw" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		var req struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		defer f.mu.Unlock()
		var res any
		switch req.Method {
		case "append":
			f.appends++
			res = 7
		case "listgroups":
			res = []any{}
			if f.state == "queue" && f.appends > 0 {
				res = []map[string]any{{"NZBID": 7, "Status": "DOWNLOADING", "FileSizeMB": 100, "RemainingSizeMB": 50}}
			}
		case "history":
			res = []any{}
			if f.state == "history" && !f.removed {
				res = []map[string]any{{"NZBID": 7, "Status": "SUCCESS/ALL", "DestDir": f.dir, "FinalDir": ""}}
			}
		case "editqueue":
			f.removed = true
			_ = os.RemoveAll(f.dir)
			res = true
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": res})
	}))
	return f
}

func TestNZBGetFallback(t *testing.T) {
	u := newURig(t)
	root := filepath.Join(u.cfg.Env.DataDir, "usenet")
	dest := filepath.Join(root, "tv-sonarr", "Ng.Show.S01E01")
	ng := newFakeNZBGet(dest)
	t.Cleanup(ng.srv.Close)
	s := u.cfg.Get()
	s.TorBox.Token = "" // no TorBox: usenet falls back to NZBGet
	s.NZBGet.URL, s.NZBGet.Username, s.NZBGet.Password, s.NZBGet.DestRoot = ng.srv.URL, "ng", "pw", root
	if err := u.cfg.Set(s); err != nil {
		t.Fatal(err)
	}
	id := u.addNZB("Ng.Show.S01E01")
	u.tick(2)
	if j, _ := u.st.Get(id); j.Backend != "nzbget" || j.State != job.Downloading {
		t.Fatalf("expected downloading on nzbget, got %s/%s (%s)", j.Backend, j.State, j.Reason)
	}
	_ = os.MkdirAll(dest, 0o755)
	_ = os.WriteFile(filepath.Join(dest, "Ng.Show.S01E01.mkv"), []byte(strings.Repeat("x", 300)), 0o644)
	_ = os.WriteFile(filepath.Join(dest, "x.nfo"), []byte("n"), 0o644)
	ng.mu.Lock()
	ng.state = "history"
	ng.mu.Unlock()
	u.tick(3)
	j, _ := u.st.Get(id)
	if j.State != job.Ready {
		t.Fatalf("expected ready, got %s (%s)", j.State, j.Reason)
	}
	if !exists(filepath.Join(j.CompleteDir, "Ng.Show.S01E01.mkv")) || exists(filepath.Join(j.CompleteDir, "x.nfo")) {
		t.Fatal("wrong files delivered")
	}
	if !ng.removed {
		t.Fatal("NZBGet's item must be removed after staging")
	}
}
