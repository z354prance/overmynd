package seerr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/z354prance/overmynd/internal/models"
)

func TestRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v1/request":
				break
			case "/api/v1/tv/12345":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"Example TV Series"}`))
				return
			default:
				t.Fatalf("path = %q", r.URL.Path)
			}

			if r.Header.Get("X-Api-Key") != "test-key" {
				t.Fatal("missing API key")
			}

			if r.URL.Query().Get("take") != "100" {
				t.Fatal("take should be 100")
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"pageInfo":{
					"pages":1,
					"pageSize":100,
					"results":1,
					"page":1
				},
				"results":[
					{
						"id":42,
						"status":2,
						"createdAt":"2026-09-20T12:00:00.000Z",
						"updatedAt":"2026-09-20T12:05:00.000Z",
						"media":{
							"id":100,
							"mediaType":"tv",
							"tmdbId":12345,
							"status":3,
							"status4k":1
						},
						"requestedBy":{
							"id":7,
							"displayName":"Example User",
							"username":"example"
						},
						"seasons":[
							{
								"id":1,
								"seasonNumber":2,
								"status":3
							},
							{
								"id":2,
								"seasonNumber":3,
								"status":3
							}
						]
					}
				]
			}`))
		},
	))
	defer server.Close()

	service := models.Service{
		ID:      6,
		Type:    models.ServiceSeerr,
		Name:    "Seerr",
		Enabled: true,
		BaseURL: server.URL,
	}

	requests, err := New().Requests(
		context.Background(),
		service,
		"test-key",
	)
	if err != nil {
		t.Fatalf("Requests: %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf(
			"len(requests) = %d, want 1",
			len(requests),
		)
	}

	item := requests[0]

	if item.ID != "6:request:42" {
		t.Fatalf("ID = %q", item.ID)
	}
	if item.Source != models.ServiceSeerr {
		t.Fatalf("Source = %q", item.Source)
	}
	if item.TMDBID != 12345 {
		t.Fatalf("TMDBID = %d", item.TMDBID)
	}
	if item.Title != "Example TV Series" {
		t.Fatalf("Title = %q", item.Title)
	}
	if item.MediaType != "tv" {
		t.Fatalf("MediaType = %q", item.MediaType)
	}
	if item.Status != "approved" {
		t.Fatalf("Status = %q", item.Status)
	}
	if item.MediaStatus != "processing" {
		t.Fatalf("MediaStatus = %q", item.MediaStatus)
	}
	if item.RequestedBy != "Example User" {
		t.Fatalf("RequestedBy = %q", item.RequestedBy)
	}
	if len(item.Seasons) != 2 ||
		item.Seasons[0] != 2 ||
		item.Seasons[1] != 3 {
		t.Fatalf("Seasons = %#v", item.Seasons)
	}
	if item.CreatedAt == nil || item.UpdatedAt == nil {
		t.Fatal("timestamps should be populated")
	}
}

func TestRequestStatus(t *testing.T) {
	tests := map[int]string{
		1:  "pending",
		2:  "approved",
		3:  "declined",
		5:  "completed",
		99: "unknown:99",
	}

	for input, expected := range tests {
		if actual := requestStatus(input); actual != expected {
			t.Fatalf(
				"requestStatus(%d) = %q, want %q",
				input,
				actual,
				expected,
			)
		}
	}
}

func TestMediaStatus(t *testing.T) {
	tests := map[int]string{
		1:  "unknown",
		2:  "pending",
		3:  "processing",
		4:  "partially_available",
		5:  "available",
		99: "unknown:99",
	}

	for input, expected := range tests {
		if actual := mediaStatus(input); actual != expected {
			t.Fatalf(
				"mediaStatus(%d) = %q, want %q",
				input,
				actual,
				expected,
			)
		}
	}
}

func TestRequestsPagination(t *testing.T) {
	callCount := 0

	movieTitles := map[string]string{
		"/api/v1/movie/1001": "Movie One",
		"/api/v1/movie/1101": "Movie Two",
	}

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/request" {
				callCount++
			}

			if title, ok := movieTitles[r.URL.Path]; ok {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"title":"` + title + `"}`))
				return
			}

			skip := r.URL.Query().Get("skip")

			w.Header().Set("Content-Type", "application/json")

			switch skip {
			case "0":
				_, _ = w.Write([]byte(`{
					"pageInfo":{
						"pages":2,
						"pageSize":100,
						"results":2,
						"page":1
					},
					"results":[
						{
							"id":1,
							"status":2,
							"media":{
								"mediaType":"movie",
								"tmdbId":1001,
								"status":3
							}
						}
					]
				}`))

			case "100":
				_, _ = w.Write([]byte(`{
					"pageInfo":{
						"pages":2,
						"pageSize":100,
						"results":2,
						"page":2
					},
					"results":[
						{
							"id":101,
							"status":2,
							"media":{
								"mediaType":"movie",
								"tmdbId":1101,
								"status":2
							}
						}
					]
				}`))

			default:
				t.Fatalf("unexpected skip = %q", skip)
			}
		},
	))
	defer server.Close()

	service := models.Service{
		ID:      6,
		Type:    models.ServiceSeerr,
		Name:    "Seerr",
		Enabled: true,
		BaseURL: server.URL,
	}

	requests, err := New().Requests(
		context.Background(),
		service,
		"test-key",
	)
	if err != nil {
		t.Fatalf("Requests: %v", err)
	}

	if callCount != 2 {
		t.Fatalf(
			"callCount = %d, want 2",
			callCount,
		)
	}

	if len(requests) != 2 {
		t.Fatalf(
			"len(requests) = %d, want 2",
			len(requests),
		)
	}

	if requests[0].SourceID != 1 {
		t.Fatalf(
			"first SourceID = %d, want 1",
			requests[0].SourceID,
		)
	}

	if requests[1].SourceID != 101 {
		t.Fatalf(
			"second SourceID = %d, want 101",
			requests[1].SourceID,
		)
	}

	if requests[1].Status != "approved" {
		t.Fatalf(
			"second Status = %q, want approved",
			requests[1].Status,
		)
	}
}

func TestActiveRequest(t *testing.T) {
	tests := []struct {
		name         string
		status       string
		availability string
		want         bool
	}{
		{
			name:         "approved processing",
			status:       "approved",
			availability: "processing",
			want:         true,
		},
		{
			name:         "approved partial",
			status:       "approved",
			availability: "partially_available",
			want:         true,
		},
		{
			name:         "pending",
			status:       "pending",
			availability: "pending",
			want:         true,
		},
		{
			name:         "completed",
			status:       "completed",
			availability: "available",
			want:         false,
		},
		{
			name:         "declined",
			status:       "declined",
			availability: "unknown",
			want:         false,
		},
		{
			name:         "available despite active status",
			status:       "approved",
			availability: "available",
			want:         false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := activeRequest(
				test.status,
				test.availability,
			)

			if got != test.want {
				t.Fatalf(
					"activeRequest(%q, %q) = %v, want %v",
					test.status,
					test.availability,
					got,
					test.want,
				)
			}
		})
	}
}
