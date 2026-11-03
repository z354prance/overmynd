package torboxarr

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/z354prance/overmynd/internal/models"
)

func TestLocalTransferQueue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api" || r.URL.RawQuery != "" {
			t.Errorf("wrong request: %s %s", r.Method, r.URL)
		}
		r.ParseForm()
		if r.PostForm.Get("apikey") != "secret" || r.PostForm.Get("mode") != "queue" || r.PostForm.Get("name") != "" {
			t.Error("invalid read-only form")
		}
		fmt.Fprint(w, `{"queue":{"kbpersec":"999999","slots":[
   {"nzo_id":"TBOX-a","filename":"Show.S01E01","status":"Downloading","mb":"100","mbleft":75,"percentage":25,"kbpersec":"512","timeleft":"0:01:00"},
   {"nzo_id":"TBOX-b","filename":"Unknown","status":"Queued","mb":"0","mbleft":"0","percentage":0},
   {"nzo_id":"TBOX-c","filename":"Moving","status":"Moving","mb":100,"mbleft":"0"},
   {"nzo_id":"TBOX-d","filename":"Done","status":"Completed"},
   {"nzo_id":"TBOX-e","filename":"Fallback","status":"Downloading","percentage":"42"},
   {"nzo_id":"TBOX-f","filename":"Bad","status":"Failed","mb":"bad","mbleft":"","percentage":"NaN"},
   {"nzo_id":"TBOX-a","filename":"Duplicate"}
  ]}}`)
	}))
	defer server.Close()
	service := models.Service{ID: 7, Type: models.ServiceTorBoxarr, Name: "TorBoxarr", BaseURL: server.URL}
	items, err := New().Queue(context.Background(), service, "secret")
	if err != nil || len(items) != 5 {
		t.Fatalf("%v %#v", err, items)
	}
	d := items[0]
	if d.Size != 100*1024*1024 || d.SizeLeft != 75*1024*1024 || d.Speed == nil || *d.Speed != 512*1024 || d.TimeLeft != "0:01:00" || d.DownloadID != "TBOX-a" || d.SourceServiceID != 7 {
		t.Fatalf("wrong local metrics: %#v", d)
	}
	if items[1].Progress != nil || items[1].Speed != nil || items[1].Size != 0 {
		t.Fatal("fabricated unknown metrics")
	}
	if items[2].Status != "moving" || items[2].SizeLeft != 0 {
		t.Fatal("moving state lost")
	}
	if items[3].Progress == nil || *items[3].Progress != 42 || items[3].Speed != nil {
		t.Fatal("percentage fallback/aggregate speed")
	}
	if items[4].Progress != nil || items[4].Size != 0 || items[4].Status != "failed" {
		t.Fatal("malformed numeric values")
	}
	if _, err := New().TestConnection(context.Background(), server.URL, "secret"); err != nil {
		t.Fatal(err)
	}
}
func TestErrorsNeverLeakCredentialsOrFollowRedirects(t *testing.T) {
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer target.Close()
	for _, code := range []int{200, 302, 401, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.WriteHeader(code)
				fmt.Fprint(w, `{"error":"secret"}`)
			}))
			defer server.Close()
			_, err := readQueue(context.Background(), server.URL, "secret")
			if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), server.URL) {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
	if leaked {
		t.Fatal("followed credential redirect")
	}
}
func TestPausedAndEmptyQueues(t *testing.T) {
	for _, body := range []string{`{"queue":{"slots":[]}}`, `{"queue":{"paused":true,"slots":[{"nzo_id":"id","status":"Downloading","kbpersec":100}]}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		items, err := New().Queue(context.Background(), models.Service{BaseURL: server.URL}, "key")
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(items) > 0 && (items[0].Status != "paused" || items[0].Speed != nil) {
			t.Fatal("paused transfer still active")
		}
	}
}
