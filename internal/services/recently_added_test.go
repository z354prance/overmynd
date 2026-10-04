package services

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/z354prance/overmynd/internal/models"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestRecentAdditionsSortedLimitedAndPrivate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/public/media/episode-id" {
			json.NewEncoder(w).Encode(map[string]string{"title": "Episode", "show_media_id": "show-id"})
			return
		}
		if r.URL.Path == "/api/v2/public/media/show-id" {
			json.NewEncoder(w).Encode(map[string]string{"title": "Fawlty Towers"})
			return
		}
		if r.Header.Get("Authorization") != "Bearer secret" || r.URL.Path != "/api/v2/public/recently-added" || r.URL.Query().Get("pageSize") != "100" {
			t.Errorf("unexpected request %s", r.URL)
		}
		kind := r.URL.Query().Get("media_type")
		data := []map[string]any{}
		for i := 1; i <= 6; i++ {
			data = append(data, map[string]any{"id": kind + string(rune('0'+i)), "server_id": "server", "title": kind, "media_id": "episode-id", "added_at": "2026-09-30T12:00:0" + string(rune('0'+i)) + "Z", "removed_at": nil, "private_token": "secret"})
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()
	m := testManager(t)
	_, err := m.Create(Input{Type: models.ServiceTracearr, Name: "Tracearr", Enabled: true, BaseURL: server.URL, Credential: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.RecentlyAdded(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 6 || len(result.Errors) != 0 || !result.Configured {
		t.Fatalf("unexpected result %+v", result)
	}
	for i, item := range result.Items {
		if item.Kind == "episode" && item.ShowTitle != "Fawlty Towers" {
			t.Fatalf("missing show title: %+v", item)
		}
		if item.Kind == "movie" && item.ShowTitle != "" {
			t.Fatal("movie has show title")
		}
		if item.Stage != "available" {
			t.Fatal(item)
		}
		if i > 0 && item.AddedAt.After(result.Items[i-1].AddedAt) {
			t.Fatal("not newest first")
		}
	}
}
func TestRecentAdditionsUnavailable(t *testing.T) {
	m := testManager(t)
	result, err := m.RecentlyAdded(context.Background())
	if err != nil || result.Configured || len(result.Items) != 0 {
		t.Fatal(result, err)
	}
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	_, err = m.Create(Input{Type: models.ServiceTracearr, Name: "Tracearr", Enabled: true, BaseURL: server.URL, Credential: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	result, err = m.RecentlyAdded(context.Background())
	if err != nil || !result.Configured || len(result.Errors) != 1 || len(result.Items) != 0 {
		t.Fatal(result, err)
	}
}

func TestGroupRecentShows(t *testing.T) {
	items := []RecentlyAddedItem{{ID: "e1", Kind: "episode", Title: "First", ShowID: "server:show1"}, {ID: "m", Kind: "movie", Title: "Movie"}, {ID: "e2", Kind: "episode", Title: "Second", ShowID: "server:show1"}, {ID: "e3", Kind: "episode", Title: "First", ShowID: "server:show2"}}
	got := groupRecentShows(items)
	if len(got) != 3 || got[0].Kind != "series" || len(got[0].Episodes) != 2 || got[1].Kind != "movie" || len(got[2].Episodes) != 1 {
		t.Fatal(got)
	}
}

func TestRecentAdditionsPagesPastBulkShow(t *testing.T) {
	for _, repeatCursor := range []bool{false, true} {
		t.Run(fmt.Sprint(repeatCursor), func(t *testing.T) {
			episodePages := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data := []map[string]any{}
				next := ""
				if r.URL.Query().Get("media_type") == "movie" {
					for i := 0; i < 6; i++ {
						data = append(data, map[string]any{"id": fmt.Sprintf("movie%d", i), "title": "Movie", "added_at": fmt.Sprintf("2026-09-20T12:00:0%dZ", i)})
					}
				} else {
					episodePages++
					if episodePages == 1 {
						for i := 0; i < 100; i++ {
							data = append(data, map[string]any{"id": fmt.Sprintf("episode%d", i), "title": "Episode", "server_id": "s", "grandparent_rating_key": "show1", "added_at": "2026-09-30T12:00:00Z"})
						}
						next = "cursor+/="
					} else {
						if r.URL.Query().Get("cursor") != "cursor+/=" {
							t.Errorf("cursor not preserved: %s", r.URL)
						}
						if repeatCursor {
							next = "cursor+/="
						} else {
							for i := 2; i <= 6; i++ {
								data = append(data, map[string]any{"id": fmt.Sprintf("show%d-episode", i), "title": "Older episode", "server_id": "s", "grandparent_rating_key": fmt.Sprint(i), "added_at": fmt.Sprintf("2026-09-2%dT12:00:00Z", i)})
							}
							next = "unused-next-page"
						}
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"data": data, "meta": map[string]any{"nextCursor": next}})
			}))
			defer server.Close()
			m := testManager(t)
			if _, err := m.Create(Input{Type: models.ServiceTracearr, Name: "Tracearr", Enabled: true, BaseURL: server.URL, Credential: "secret"}); err != nil {
				t.Fatal(err)
			}
			result, err := m.RecentlyAdded(context.Background())
			if err != nil || episodePages != 2 || len(result.Items) != 6 {
				t.Fatalf("pages=%d result=%+v err=%v", episodePages, result, err)
			}
			if repeatCursor {
				if len(result.Errors) != 1 {
					t.Fatal("missing truncated-history error")
				}
				return
			}
			if len(result.Errors) != 0 {
				t.Fatal(result.Errors)
			}
			for _, item := range result.Items {
				if item.Kind != "series" {
					t.Fatalf("older movie crowded out newer show: %+v", item)
				}
			}
			if len(result.Items[0].Episodes) != 100 {
				t.Fatal("bulk show was not grouped")
			}
		})
	}
}

func TestRecentPosterPath(t *testing.T) {
	for _, serverType := range []string{"emby", "jellyfin"} {
		got := recentPosterPath("server", serverType, "episode", "show", "episode")
		u, err := url.Parse(got)
		if err != nil || u.Query().Get("server") != "server" {
			t.Fatal(got, err)
		}
		want := "/Items/show/Images/Primary"
		if serverType == "plex" {
			want = "/library/metadata/show/thumb"
		}
		if u.Query().Get("url") != want {
			t.Fatal(got)
		}
	}
	if recentPosterPath("s", "unknown", "id", "", "movie") != "" {
		t.Fatal("unsupported server")
	}
	if recentPosterPath("", "emby", "id", "", "movie") != "" {
		t.Fatal("missing server")
	}
}
