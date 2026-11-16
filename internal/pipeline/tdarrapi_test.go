package pipeline_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/z354prance/undermynd/internal/job"
)

type fakeTdarr struct {
	mu       sync.Mutex
	srv      *httptest.Server
	records  map[string]map[string]any
	workers  map[string]map[string]any
	scans    []string
	badKey   bool
	keyCalls int
}

func newFakeTdarr() *fakeTdarr {
	f := &fakeTdarr{records: map[string]map[string]any{}, workers: map[string]map[string]any{}}
	mux := http.NewServeMux()
	chk := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("x-api-key") != "tapi_key" {
			http.Error(w, "no", http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("/api/v2/cruddb", func(w http.ResponseWriter, r *http.Request) {
		if !chk(w, r) {
			return
		}
		var req struct {
			Data struct {
				Collection, Mode, DocID string
			} `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		defer f.mu.Unlock()
		if rec, ok := f.records[req.Data.DocID]; ok && req.Data.Collection == "FileJSONDB" && req.Data.Mode == "getById" {
			_ = json.NewEncoder(w).Encode(rec)
			return
		}
		fmt.Fprint(w, "null")
	})
	mux.HandleFunc("/api/v2/get-nodes", func(w http.ResponseWriter, r *http.Request) {
		if !chk(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		ws := map[string]any{}
		for k, v := range f.workers {
			ws[k] = v
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"n1": map[string]any{"_id": "n1", "nodeName": "node-a", "workers": ws}})
	})
	mux.HandleFunc("/api/v2/scan-files", func(w http.ResponseWriter, r *http.Request) {
		if !chk(w, r) {
			return
		}
		var req struct {
			Data struct {
				ScanConfig struct {
					DBID        string `json:"dbID"`
					ArrayOrPath string `json:"arrayOrPath"`
				} `json:"scanConfig"`
			} `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.scans = append(f.scans, req.Data.ScanConfig.DBID+":"+req.Data.ScanConfig.ArrayOrPath)
		f.mu.Unlock()
		fmt.Fprint(w, "{}")
	})
	f.srv = httptest.NewServer(mux)
	return f
}

// setupTranscode makes a TorBox usenet job that has finished downloading and reached the Tdarr input folder.
func setupTranscode(t *testing.T, policy string) (*urig, *fakeTdarr, string, string) {
	u := newURig(t)
	td := newFakeTdarr()
	t.Cleanup(td.srv.Close)
	s := u.cfg.Get()
	s.Categories[0].Transcode = true
	s.Categories[0].OnTranscodeFail = policy
	s.Tdarr.URL, s.Tdarr.APIKey, s.Tdarr.LibraryID, s.Tdarr.MaxInFlight = td.srv.URL, "tapi_key", "lib1", 1
	if err := u.cfg.Set(s); err != nil {
		t.Fatal(err)
	}
	id := u.addNZB("Api.Show.S01E01")
	u.tick(2)
	u.tb.finish("101", map[string]string{"Api.Show.S01E01/Api.Show.S01E01.mkv": mkvHead + strings.Repeat("v", 4000)})
	u.tick(3)
	if j, _ := u.st.Get(id); j.State != job.QueuedTranscode {
		t.Fatalf("expected queued_transcode, got %s (%s)", j.State, j.Reason)
	}
	return u, td, id, filepath.Join(s.Tdarr.InDir, id, "Api.Show.S01E01.mkv")
}

func TestTdarrAPIProgressAndSuccess(t *testing.T) {
	u, td, id, in := setupTranscode(t, "")
	td.mu.Lock()
	if len(td.scans) != 1 || !strings.HasPrefix(td.scans[0], "lib1:") {
		t.Fatalf("expected one scan of the input folder in the dedicated library, got %v", td.scans)
	}
	td.workers["w1"] = map[string]any{"_id": "w1", "idle": false, "file": in, "percentage": 40.0, "status": "Transcoding"}
	td.mu.Unlock()
	u.tick(1)
	j, _ := u.st.Get(id)
	if j.State != job.Transcoding || j.TranscodeNode != "node-a" || j.TranscodeProgress < 0.39 || j.TranscodeProgress > 0.41 {
		t.Fatalf("expected transcoding with the worker's node and progress, got %s node=%q p=%v", j.State, j.TranscodeNode, j.TranscodeProgress)
	}
	if o := job.Overall(j); o < 0.91 || o > 0.93 {
		t.Fatalf("overall progress should include transcode progress, got %v", o)
	}
	// the worker finishes and Tdarr writes the output
	td.mu.Lock()
	delete(td.workers, "w1")
	td.records[in] = map[string]any{"_id": in, "DB": "lib1", "TranscodeDecisionMaker": "Transcode success"}
	td.mu.Unlock()
	out := filepath.Join(u.cfg.Get().Tdarr.OutDir, id)
	_ = os.MkdirAll(out, 0o755)
	small := mkvHead + strings.Repeat("s", 1500)
	_ = os.WriteFile(filepath.Join(out, "Api.Show.S01E01.mkv"), []byte(small), 0o644)
	u.tick(6)
	j, _ = u.st.Get(id)
	if j.State != job.Ready {
		t.Fatalf("expected ready, got %s (%s)", j.State, j.Reason)
	}
}

func TestTdarrAPIFailureFailsGrab(t *testing.T) {
	u, td, id, in := setupTranscode(t, "")
	td.mu.Lock()
	td.records[in] = map[string]any{"_id": in, "TranscodeDecisionMaker": "Transcode error"}
	td.mu.Unlock()
	u.tick(2)
	j, _ := u.st.Get(id)
	if j.State != job.Failed || !strings.Contains(j.Reason, "Tdarr reported") {
		t.Fatalf("a Tdarr error must fail the grab at once, got %s (%s)", j.State, j.Reason)
	}
}

func TestTdarrAPIFailurePassthrough(t *testing.T) {
	u, td, id, in := setupTranscode(t, "passthrough")
	td.mu.Lock()
	td.records[in] = map[string]any{"_id": in, "TranscodeDecisionMaker": "Transcode error"}
	td.mu.Unlock()
	u.tick(3)
	j, _ := u.st.Get(id)
	if j.State != job.Ready {
		t.Fatalf("passthrough must hand back the original, got %s (%s)", j.State, j.Reason)
	}
	if b, _ := os.ReadFile(filepath.Join(j.CompleteDir, "Api.Show.S01E01.mkv")); !strings.HasPrefix(string(b), mkvHead) || len(b) != len(mkvHead)+4000 {
		t.Fatal("the original file must be what is handed back")
	}
}

func TestTdarrNotRequiredKeepsOriginal(t *testing.T) {
	u, td, id, in := setupTranscode(t, "")
	td.mu.Lock()
	td.records[in] = map[string]any{"_id": in, "TranscodeDecisionMaker": "Not required"}
	td.mu.Unlock()
	u.tick(8)
	j, _ := u.st.Get(id)
	if j.State != job.Ready {
		t.Fatalf("a file Tdarr says needs no work should be handed back as is, got %s (%s)", j.State, j.Reason)
	}
}

func TestTdarrInFlightCap(t *testing.T) {
	u, _, id, _ := setupTranscode(t, "")
	// a second job finishes downloading while the first holds the only slot
	id2 := u.addNZB("Second.Show.S01E02")
	u.tick(2)
	u.tb.finish("102", map[string]string{"Second.Show.S01E02/Second.Show.S01E02.mkv": mkvHead + strings.Repeat("v", 3000)})
	u.tick(3)
	j2, _ := u.st.Get(id2)
	if j2.State != job.Downloaded || !strings.Contains(j2.Reason, "free Tdarr slot") {
		t.Fatalf("the second job must wait for a slot, got %s (%s)", j2.State, j2.Reason)
	}
	if j1, _ := u.st.Get(id); j1.State != job.QueuedTranscode {
		t.Fatalf("first job should still be with Tdarr, got %s", j1.State)
	}
}

// A hardware AV1 encode can sit at one percentage for a long time. While a Tdarr worker
// holds the file, Undermynd must not pull it back to staging and hand the original to the
// arr, whatever the category's failure policy says.
func TestTdarrWorkerHoldingFileIsNeverHandedBack(t *testing.T) {
	u, td, id, in := setupTranscode(t, "passthrough")
	td.mu.Lock()
	td.workers["w1"] = map[string]any{"_id": "w1", "idle": false, "file": in, "percentage": 12.0, "status": "Transcoding"}
	td.mu.Unlock()
	u.tick(90) // far past the old 10 minute stall limit, with the percentage flat
	j, _ := u.st.Get(id)
	if j.State == job.Ready || j.State == job.Failed || j.State == job.Imported {
		t.Fatalf("a job with a live Tdarr worker must stay with Tdarr, got %s (%s)", j.State, j.Reason)
	}
	if _, err := os.Stat(in); err != nil {
		t.Fatalf("the file must stay in the Tdarr input folder while it is encoded: %v", err)
	}
	// once the worker is gone and Tdarr reports an error, the category policy applies
	td.mu.Lock()
	delete(td.workers, "w1")
	td.records[in] = map[string]any{"_id": in, "TranscodeDecisionMaker": "Transcode error"}
	td.mu.Unlock()
	u.tick(4)
	if j, _ = u.st.Get(id); j.State != job.Ready {
		t.Fatalf("after the worker ends in error, passthrough should hand back, got %s (%s)", j.State, j.Reason)
	}
}
