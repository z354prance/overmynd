package torboxarr

import (
	"context"
	"fmt"
	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTorrentQueue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			r.ParseForm()
			if r.Method != "POST" || r.Form.Get("username") != "admin" || r.Form.Get("password") != "secret" {
				t.Error("wrong login")
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "session", Path: "/"})
			fmt.Fprint(w, "Ok.")
			return
		}
		if r.URL.Path != "/api/v2/torrents/info" || r.Method != "GET" {
			t.Error("unexpected endpoint")
		}
		if cookie, err := r.Cookie("SID"); err != nil || cookie.Value != "session" {
			t.Error("missing session")
		}
		fmt.Fprint(w, `[
   {"hash":"a","name":"Active","state":"downloading","size":1000,"amount_left":400,"dlspeed":200,"eta":2},
   {"hash":"b","name":"Queued","state":"queuedDL","size":0,"progress":0},
   {"hash":"c","name":"Verify","state":"checkingResumeData","size":1000,"amount_left":0},
   {"hash":"d","name":"Done","state":"pausedUP","progress":1},
   {"hash":"e","name":"Failed","state":"error"}
  ]`)
	}))
	defer server.Close()
	credential, _ := integrations.EncodeUsernamePassword("admin", "secret")
	items, err := NewTorrent().Queue(context.Background(), models.Service{ID: 4, BaseURL: server.URL, Name: "TorBoxarr Torrents"}, credential)
	if err != nil || len(items) != 4 {
		t.Fatalf("%v %#v", err, items)
	}
	if items[0].Source != models.ServiceTorBoxarrTorrent || items[0].SizeLeft != 400 || items[0].Speed == nil || *items[0].Speed != 200 || items[0].TimeLeft != "2s" {
		t.Fatal(items[0])
	}
	if items[1].Status != "queued" || items[1].Progress != nil || items[1].Speed != nil {
		t.Fatal(items[1])
	}
	if items[2].Status != "verifying" || items[3].Status != "failed" {
		t.Fatal(items)
	}
}
func TestTorrentCredentialsProtected(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer target.Close()
	credential, _ := integrations.EncodeUsernamePassword("admin", "secret")
	for _, code := range []int{200, 307, 403, 500} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", target.URL)
			w.WriteHeader(code)
			fmt.Fprint(w, "secret")
		}))
		_, err := NewTorrent().TestConnection(context.Background(), server.URL, credential)
		server.Close()
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe error: %v", err)
		}
	}
	if hit {
		t.Fatal("credentials redirected")
	}
}
