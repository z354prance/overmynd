package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/z354prance/overmynd/internal/auth"
	"github.com/z354prance/overmynd/internal/database"
	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/integrations/tracearr"
	"github.com/z354prance/overmynd/internal/models"
	"github.com/z354prance/overmynd/internal/services"
)

func TestPlaybackArtworkProxy(t *testing.T) {
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jxQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	posterCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/public/streams":
			if r.Header.Get("Authorization") != "Bearer private-token" {
				t.Error("missing stream authentication")
			}
			fmt.Fprint(w, `{"data":[{"id":"session","media_title":"Test","poster_url":"/api/v1/images/proxy?server=abc&url=%2Flibrary%2F1"}]}`)
		case "/api/v1/images/proxy":
			posterCalls++
			if r.Header.Get("Authorization") != "" {
				t.Error("poster leaked token")
			}
			w.Write(png)
		default:
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	manager := services.NewManager(db)
	_, err = manager.Create(services.Input{Type: models.ServiceTracearr, Name: "Tracearr", Enabled: true, BaseURL: upstream.URL, Credential: "private-token"})
	if err != nil {
		t.Fatal(err)
	}
	registry := integrations.NewRegistry()
	registry.Register(tracearr.New())
	handler := NewRouter(manager, registry, auth.NewManager(db))
	w := request(t, handler, "", "GET", "/api/v1/playback", "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var result services.PlaybackResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Sessions) != 1 {
		t.Fatal("missing session")
	}
	poster := result.Sessions[0].PosterURL
	if !strings.HasPrefix(poster, "/api/v1/playback/poster?") || strings.Contains(poster, "private-token") || strings.Contains(poster, upstream.URL) {
		t.Fatalf("unsafe poster: %s", poster)
	}
	w = request(t, handler, "", "GET", poster, "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("image: %d %s", w.Code, w.Body)
	}
	w = request(t, handler, "", "GET", poster+"tampered", "")
	if w.Code != 404 || posterCalls != 1 {
		t.Fatal("invalid signature accepted")
	}
	a := &API{}
	for _, path := range []string{"https://attacker.example/image", "//attacker.example/image", "/api/v1/settings", "/api/v1/images/proxy/../settings", "/api/v1/images/proxy#x"} {
		if a.posterURL(1, path) != "" {
			t.Errorf("signed unsafe URL: %s", path)
		}
	}
}
