package services

import (
	"path/filepath"
	"testing"

	"github.com/z354prance/overmynd/internal/database"
	"github.com/z354prance/overmynd/internal/models"
)

func testManager(t *testing.T) *Manager {
	t.Helper()

	db, err := database.Open(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	if err := db.Migrate(); err != nil {
		db.Close()
		t.Fatalf("migrate database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return NewManager(db)
}

func TestCreateService(t *testing.T) {
	manager := testManager(t)

	service, err := manager.Create(Input{
		Type:       models.ServiceSonarr,
		Name:       " Sonarr ",
		Enabled:    true,
		BaseURL:    "http://10.0.0.20:8989/",
		Credential: "secret",
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	if service.Name != "Sonarr" {
		t.Fatalf("unexpected name: %q", service.Name)
	}

	if service.BaseURL != "http://10.0.0.20:8989" {
		t.Fatalf("unexpected URL: %q", service.BaseURL)
	}

	if !service.HasCredential {
		t.Fatal("expected credential")
	}
}

func TestRejectUnsupportedService(t *testing.T) {
	manager := testManager(t)

	_, err := manager.Create(Input{
		Type:    models.ServiceType("unknown"),
		Name:    "Unknown",
		Enabled: true,
		BaseURL: "http://localhost:1234",
	})

	if err == nil {
		t.Fatal("unsupported service should fail")
	}
}

func TestRejectInvalidURL(t *testing.T) {
	manager := testManager(t)

	_, err := manager.Create(Input{
		Type:    models.ServiceRadarr,
		Name:    "Radarr",
		Enabled: true,
		BaseURL: "not-a-url",
	})

	if err == nil {
		t.Fatal("invalid URL should fail")
	}
}
