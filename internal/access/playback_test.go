package access

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/z354prance/overmynd/internal/models"
)

func TestPlaybackLibraryPrivacy(t *testing.T) {
	for _, mode := range []string{"ok", "unauthorized", "empty", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.Header.Get("X-Emby-Token") != "secret" || r.Header.Get("Accept") != "application/json" {
					t.Error("incorrect authenticated read")
				}
				if mode == "unauthorized" {
					w.WriteHeader(401)
					return
				}
				if mode == "malformed" {
					fmt.Fprint(w, "invalid")
					return
				}
				if mode == "empty" {
					fmt.Fprint(w, "[]")
					return
				}
				switch r.URL.Path {
				case "/Library/VirtualFolders":
					fmt.Fprint(w, `[{"ItemId":"5"},{"ItemId":"9"}]`)
				case "/Items/484415/Ancestors":
					fmt.Fprint(w, `[{"Id":"9"},{"Id":"root"}]`)
				case "/Items/123/Ancestors":
					fmt.Fprint(w, `[{"Id":"season"},{"Id":"5"},{"Id":"root"}]`)
				case "/Items/456/Ancestors":
					fmt.Fprint(w, `[{"Id":"root"}]`)
				default:
					t.Errorf("unexpected lookup %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			sessions := []models.PlaybackSession{
				{ID: "hidden-known", ServerType: "emby", LibraryID: "9"},
				{ID: "tv-known", ServerType: "emby", LibraryID: "5"},
				{ID: "private-missing-library", ServerType: "emby", RatingKey: "484415"},
				{ID: "tv-resolved", ServerType: "Emby", RatingKey: "123"},
				{ID: "unresolved-root", ServerType: "emby", RatingKey: "456"},
				{ID: "no-item", ServerType: "emby"},
				{ID: "invalid-item", ServerType: "emby", RatingKey: "../Users"},
				{ID: "unknown-server"},
				{ID: "plex", ServerType: "plex", LibraryID: "9"},
			}
			result := filterPlayback(context.Background(), Settings{EmbyURL: server.URL, EmbyKey: "secret"}, sessions, map[string]bool{"9": true})
			ids := []string{}
			for _, session := range result {
				ids = append(ids, session.ID)
			}
			expected := []string{"tv-known", "plex"}
			if mode == "ok" {
				expected = []string{"tv-known", "tv-resolved", "plex"}
			}
			if !reflect.DeepEqual(ids, expected) {
				t.Fatalf("public sessions %v, want %v", ids, expected)
			}
		})
	}
}

func TestPlaybackPrivacyDisabledAndMissingCredentials(t *testing.T) {
	sessions := []models.PlaybackSession{{ID: "unknown", ServerType: "emby", RatingKey: "123"}}
	// Disabled filtering must not require a settings database.
	manager := &Manager{}
	if got := manager.FilterPlayback(context.Background(), sessions, " , "); len(got) != 1 {
		t.Fatal("disabled filter changed sessions")
	}
	if got := filterPlayback(context.Background(), Settings{}, sessions, map[string]bool{"9": true}); len(got) != 0 {
		t.Fatal("unknown private session exposed without credentials")
	}
}
