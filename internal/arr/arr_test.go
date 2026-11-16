package arr_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/z354prance/undermynd/internal/arr"
)

func server(t *testing.T, key string) *httptest.Server {
	mux := http.NewServeMux()
	guard := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Api-Key") != key {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/api/v3/system/status", guard(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"appName":"Sonarr","version":"4.0.1"}`))
	}))
	mux.HandleFunc("/api/v3/history", guard(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"records":[]}`))
	}))
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func TestCheckReportsTheArrAndCatchesABadKey(t *testing.T) {
	s := server(t, "good")
	app, ver, err := arr.New("sonarr", s.URL, "good").Check(context.Background())
	if err != nil || app != "Sonarr" || ver != "4.0.1" {
		t.Fatalf("good key: %q %q %v", app, ver, err)
	}
	if _, _, err := arr.New("sonarr", s.URL, "bad").Check(context.Background()); err == nil || !strings.Contains(err.Error(), "refused the API key") {
		t.Fatalf("a wrong key must say so, got %v", err)
	}
}

func TestFailDownloadBlocklistsTheQueueEntry(t *testing.T) {
	var deleted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/queue":
			w.Write([]byte(`{"records":[{"id":7,"downloadId":"ABC"},{"id":8,"downloadId":"other"}]}`))
		case r.Method == http.MethodDelete:
			deleted = r.URL.Path + "?" + r.URL.RawQuery
		}
	}))
	defer srv.Close()
	found, err := arr.New("sonarr", srv.URL, "k").FailDownload(context.Background(), "abc")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if deleted != "/api/v3/queue/7?removeFromClient=true&blocklist=true&skipRedownload=false" {
		t.Fatalf("deleted %q", deleted)
	}
}

func TestGrabbedResolutionReadsTheGrabbedQuality(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/history", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("downloadId") != "nzo_1" {
			t.Errorf("wrong download id: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"records":[{"eventType":"downloadFolderImported","quality":{"quality":{"resolution":480}}},{"eventType":"grabbed","quality":{"quality":{"resolution":1080}}}]}`))
	})
	s := httptest.NewServer(mux)
	defer s.Close()
	if got, err := arr.New("sonarr", s.URL, "k").GrabbedResolution(context.Background(), "nzo_1"); err != nil || got != 1080 {
		t.Fatalf("got %d %v", got, err)
	}
	if got, _ := arr.New("lidarr", s.URL, "k").GrabbedResolution(context.Background(), "nzo_1"); got != 0 {
		t.Fatalf("music has no resolution, got %d", got)
	}
}
