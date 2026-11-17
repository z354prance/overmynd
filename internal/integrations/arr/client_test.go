package arr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSystemStatus(t *testing.T) {
	const apiKey = "test-api-key"

	server := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			if r.URL.Path != "/api/v3/system/status" {
				t.Fatalf(
					"unexpected path: %s",
					r.URL.Path,
				)
			}

			if r.Header.Get("X-Api-Key") != apiKey {
				t.Fatal("API key header missing")
			}

			w.Header().Set(
				"Content-Type",
				"application/json",
			)

			_, _ = w.Write([]byte(`{
				"appName":"Radarr",
				"instanceName":"Radarr",
				"version":"6.0.0"
			}`))
		}),
	)
	defer server.Close()

	client := NewClient("v3")

	status, err := client.SystemStatus(
		context.Background(),
		server.URL,
		apiKey,
	)
	if err != nil {
		t.Fatalf("system status: %v", err)
	}

	if status.AppName != "Radarr" {
		t.Fatalf(
			"unexpected app name: %q",
			status.AppName,
		)
	}

	if status.Version != "6.0.0" {
		t.Fatalf(
			"unexpected version: %q",
			status.Version,
		)
	}
}

func TestSystemStatusRequiresAPIKey(t *testing.T) {
	client := NewClient("v3")

	_, err := client.SystemStatus(
		context.Background(),
		"http://localhost:7878",
		"",
	)

	if err == nil {
		t.Fatal("missing API key should fail")
	}
}

func TestEndpointVersions(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{
			version: "v3",
			want:    "http://localhost:7878/api/v3/system/status",
		},
		{
			version: "v1",
			want:    "http://localhost:7878/api/v1/system/status",
		},
	}

	for _, test := range tests {
		client := NewClient(test.version)

		got, err := client.Endpoint(
			"http://localhost:7878/",
			"system/status",
		)
		if err != nil {
			t.Fatalf(
				"build %s endpoint: %v",
				test.version,
				err,
			)
		}

		if got != test.want {
			t.Fatalf(
				"endpoint = %q, want %q",
				got,
				test.want,
			)
		}
	}
}
