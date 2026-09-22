package api

import (
	"testing"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

func TestServiceInputQBittorrentCredentials(t *testing.T) {
	input, err := serviceInput(serviceRequest{
		Type:     models.ServiceQBittorrent,
		Name:     "qBittorrent",
		Enabled:  true,
		BaseURL:  "http://10.0.0.10:8080",
		Username: "admin",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("serviceInput: %v", err)
	}

	credential, err := integrations.DecodeUsernamePassword(
		input.Credential,
	)
	if err != nil {
		t.Fatalf("DecodeUsernamePassword: %v", err)
	}

	if credential.Username != "admin" {
		t.Fatalf(
			"Username = %q, want admin",
			credential.Username,
		)
	}

	if credential.Password != "secret" {
		t.Fatalf(
			"Password = %q, want secret",
			credential.Password,
		)
	}
}

func TestServiceInputAPIKey(t *testing.T) {
	input, err := serviceInput(serviceRequest{
		Type:       models.ServiceRadarr,
		Name:       "Radarr",
		Enabled:    true,
		BaseURL:    "http://10.0.0.10:7878",
		Credential: "api-key",
	})
	if err != nil {
		t.Fatalf("serviceInput: %v", err)
	}

	if input.Credential != "api-key" {
		t.Fatalf(
			"Credential = %q, want api-key",
			input.Credential,
		)
	}
}
