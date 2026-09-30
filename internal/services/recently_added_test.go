package services

import (
	"context"
	"encoding/json"
	"github.com/z354prance/overmynd/internal/models"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecentAdditionsSortedLimitedAndPrivate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" || r.URL.Path != "/api/v2/public/recently-added" || r.URL.Query().Get("pageSize") != "5" {
			t.Errorf("unexpected request %s", r.URL)
		}
		kind := r.URL.Query().Get("media_type")
		data := []map[string]any{}
		for i := 1; i <= 5; i++ {
			data = append(data, map[string]any{"id": kind + string(rune('0'+i)), "server_id": "server", "title": kind, "added_at": "2026-09-30T12:00:0" + string(rune('0'+i)) + "Z", "removed_at": nil, "private_token": "secret"})
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
	if len(result.Items) != 5 || len(result.Errors) != 0 || !result.Configured {
		t.Fatalf("unexpected result %+v", result)
	}
	for i, item := range result.Items {
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
