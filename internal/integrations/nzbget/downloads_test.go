package nzbget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

func TestQueue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if !ok {
				t.Fatal("missing basic auth")
			}
			if username != "nzbuser" || password != "nzbpass" {
				t.Fatal("unexpected credentials")
			}

			var request rpcRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode request: %v", err)
			}

			if request.Method != "listgroups" {
				t.Fatalf(
					"method = %q, want listgroups",
					request.Method,
				)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"version":"1.1",
				"result":[
					{
						"NZBID":42,
						"NZBName":"Example Download",
						"DestDir":"/downloads/example",
						"Status":"DOWNLOADING",
						"FileSizeMB":1000,
						"RemainingSizeMB":250
					},
					{
						"NZBID":43,
						"NZBName":"Completed Download",
						"DestDir":"/downloads/completed",
						"Status":"SUCCESS",
						"FileSizeMB":500,
						"RemainingSizeMB":0
					}
				],
				"error":null,
				"id":1
			}`))
		},
	))
	defer server.Close()

	credential, err := integrations.EncodeUsernamePassword(
		"nzbuser",
		"nzbpass",
	)
	if err != nil {
		t.Fatalf("encode credentials: %v", err)
	}

	service := models.Service{
		ID:      5,
		Type:    models.ServiceNZBGet,
		Name:    "NZBGet",
		Enabled: true,
		BaseURL: server.URL,
	}

	items, err := New().Queue(
		context.Background(),
		service,
		credential,
	)
	if err != nil {
		t.Fatalf("Queue: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}

	item := items[0]

	if item.Source != models.ServiceNZBGet {
		t.Fatalf("source = %q, want nzbget", item.Source)
	}
	if item.DownloadID != "42" {
		t.Fatalf(
			"download ID = %q, want 42",
			item.DownloadID,
		)
	}
	if item.Protocol != "usenet" {
		t.Fatalf(
			"protocol = %q, want usenet",
			item.Protocol,
		)
	}
	if item.Status != "DOWNLOADING" {
		t.Fatalf(
			"status = %q, want DOWNLOADING",
			item.Status,
		)
	}
	if item.Size != 1000*1024*1024 {
		t.Fatalf(
			"size = %d, want %d",
			item.Size,
			int64(1000*1024*1024),
		)
	}
	if item.SizeLeft != 250*1024*1024 {
		t.Fatalf(
			"size left = %d, want %d",
			item.SizeLeft,
			int64(250*1024*1024),
		)
	}
}

func TestCompletedNZBGetGroupFiltered(t *testing.T) {
	if !isCompletedGroup(group{
		Status: "SUCCESS",
	}) {
		t.Fatal("SUCCESS group should be filtered")
	}

	if !isCompletedGroup(group{
		Status: "DELETED",
	}) {
		t.Fatal("DELETED group should be filtered")
	}

	if isCompletedGroup(group{
		Status: "DOWNLOADING",
	}) {
		t.Fatal("DOWNLOADING group should remain active")
	}
}
