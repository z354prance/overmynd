package backend_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/z354prance/undermynd/internal/backend"
)

// The list endpoint can leave out a real item (a cap, or a stale cache); asking
// for it by ID must still find it so a good download is not failed.
func TestStatusFindsItemMissingFromTheList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/api/usenet/mylist", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("bypass_cache") != "true" {
			t.Error("every list call must bypass TorBox's cache")
		}
		switch r.URL.Query().Get("id") {
		case "":
			_, _ = w.Write([]byte(`{"success":true,"data":[{"id":1,"download_state":"cached","download_finished":true,"files":[{"id":1,"name":"a.mkv","size":5}]}]}`))
		case "555":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":555,"progress":1,"download_state":"cached","download_finished":true,"files":[{"id":9,"name":"x.mkv","size":7}]}}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"data":null}`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	lim := backend.NewLimiter(filepath.Join(t.TempDir(), "l.json"), 60, time.Millisecond)
	lim.APIGap = 0
	tb := backend.NewTorBox(srv.URL, "tok", lim)

	it, err := tb.Status(context.Background(), "555")
	if err != nil || it.Status != backend.Complete {
		t.Fatalf("item outside the list should be found by ID: %+v %v", it, err)
	}
	if files, err := tb.Files(context.Background(), "555"); err != nil || len(files) != 1 || files[0].Path != "x.mkv" {
		t.Fatalf("files by ID: %+v %v", files, err)
	}
	if it, err := tb.Status(context.Background(), "999"); err != nil || it.Status != backend.Missing {
		t.Fatalf("an item TorBox does not have is missing: %+v %v", it, err)
	}
}

// TorBox answers the list of an empty account with HTTP 404 ITEM_NOT_FOUND.
// That means "nothing there", not a failure, so removal and status can finish.
func TestEmptyAccountListIsNotAnError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/api/usenet/mylist", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"error":"ITEM_NOT_FOUND","detail":"No Usenet downloads found for this user.","data":null}`))
	})
	mux.HandleFunc("/v1/api/usenet/controlusenetdownload", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":null}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	lim := backend.NewLimiter(filepath.Join(t.TempDir(), "l.json"), 60, time.Millisecond)
	lim.APIGap = 0
	tb := backend.NewTorBox(srv.URL, "tok", lim)
	if it, err := tb.Status(context.Background(), "7"); err != nil || it.Status != backend.Missing {
		t.Fatalf("empty account: %+v %v", it, err)
	}
	if err := tb.Remove(context.Background(), "7", true); err != nil {
		t.Fatalf("remove on an empty account: %v", err)
	}
}

// The list can show an item as still downloading after it finished; the item
// asked for by ID is the live one and wins.
func TestStatusPrefersTheLiveItemOverAStaleList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/api/usenet/mylist", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") == "" {
			_, _ = w.Write([]byte(`{"success":true,"data":[{"id":7,"progress":0.36,"download_state":"downloading","download_finished":false,"files":[]}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"progress":1,"download_state":"cached","download_finished":true,"files":[{"id":1,"name":"a.mkv","size":5}]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	lim := backend.NewLimiter(filepath.Join(t.TempDir(), "l.json"), 60, time.Millisecond)
	lim.APIGap = 0
	tb := backend.NewTorBox(srv.URL, "tok", lim)
	if it, err := tb.Status(context.Background(), "7"); err != nil || it.Status != backend.Complete {
		t.Fatalf("the live item should win over a stale list: %+v %v", it, err)
	}
}

// An unfinished item TorBox is not working on is flagged; a queued one is not.
func TestStatusFlagsInactiveItems(t *testing.T) {
	state := "processing"
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/api/usenet/mylist", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"progress":0.8,"active":false,"download_state":"` + state + `","download_finished":false,"files":[]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	lim := backend.NewLimiter(filepath.Join(t.TempDir(), "l.json"), 60, time.Millisecond)
	lim.APIGap = 0
	tb := backend.NewTorBox(srv.URL, "tok", lim)
	if it, _ := tb.Status(context.Background(), "7"); !it.Inactive {
		t.Fatalf("inactive item not flagged: %+v", it)
	}
	state = "queued"
	if it, _ := tb.Status(context.Background(), "7"); it.Inactive {
		t.Fatalf("queued item must not be flagged: %+v", it)
	}
}
