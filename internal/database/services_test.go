package database

import (
	"path/filepath"
	"testing"

	"github.com/z354prance/overmynd/internal/models"
)

func testDatabase(t *testing.T) *Database {
	t.Helper()

	db, err := Open(filepath.Join(t.TempDir(), "config"))
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

	return db
}

func TestServiceRepository(t *testing.T) {
	db := testDatabase(t)

	input := ServiceInput{
		Type:       models.ServiceRadarr,
		Name:       "Radarr",
		Enabled:    true,
		BaseURL:    "http://10.0.0.10:7878",
		Credential: "super-secret-api-key",
	}

	id, err := db.CreateService(input)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	service, err := db.GetService(id)
	if err != nil {
		t.Fatalf("get service: %v", err)
	}

	if service.Type != models.ServiceRadarr {
		t.Fatalf("unexpected type: %s", service.Type)
	}

	if !service.HasCredential {
		t.Fatal("credential should be reported as present")
	}

	credential, err := db.GetServiceCredential(id)
	if err != nil {
		t.Fatalf("get credential: %v", err)
	}

	if credential != input.Credential {
		t.Fatal("credential mismatch")
	}

	services, err := db.ListServices()
	if err != nil {
		t.Fatalf("list services: %v", err)
	}

	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}

	update := ServiceInput{
		Type:    models.ServiceRadarr,
		Name:    "Movies",
		Enabled: false,
		BaseURL: "http://10.0.0.11:7878",
	}

	if err := db.UpdateService(id, update, false); err != nil {
		t.Fatalf("update service: %v", err)
	}

	credential, err = db.GetServiceCredential(id)
	if err != nil {
		t.Fatalf("get credential after update: %v", err)
	}

	if credential != input.Credential {
		t.Fatal("credential changed unexpectedly")
	}

	if err := db.DeleteService(id); err != nil {
		t.Fatalf("delete service: %v", err)
	}

	services, err = db.ListServices()
	if err != nil {
		t.Fatalf("list services after delete: %v", err)
	}

	if len(services) != 0 {
		t.Fatalf("expected no services, got %d", len(services))
	}
}
