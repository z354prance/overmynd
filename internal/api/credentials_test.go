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

func TestTorBoxarrCredentialTypes(t *testing.T) {
	sab, err := serviceInput(serviceRequest{Type: models.ServiceTorBoxarr, Credential: "sab-secret"})
	if err != nil || sab.Credential != "sab-secret" {
		t.Fatal("SAB credential changed")
	}
	torrent, err := serviceInput(serviceRequest{Type: models.ServiceTorBoxarrTorrent, Username: "admin", Password: "torrent-secret"})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := integrations.DecodeUsernamePassword(torrent.Credential)
	if err != nil || auth.Username != "admin" || auth.Password != "torrent-secret" {
		t.Fatal("torrent login not encoded")
	}
	blank, err := serviceInput(serviceRequest{Type: models.ServiceTorBoxarrTorrent})
	if err != nil || blank.Credential != "" {
		t.Fatal("blank edit must preserve saved credentials")
	}
}
