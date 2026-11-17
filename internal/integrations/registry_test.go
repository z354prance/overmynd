package integrations

import (
	"context"
	"testing"

	"github.com/z354prance/overmynd/internal/models"
)

type testIntegration struct {
	serviceType models.ServiceType
}

func (t testIntegration) Type() models.ServiceType {
	return t.serviceType
}

func (t testIntegration) TestConnection(
	_ context.Context,
	_ string,
	_ string,
) (ConnectionResult, error) {
	return ConnectionResult{
		OK:      true,
		Message: "ok",
	}, nil
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	integration := testIntegration{
		serviceType: models.ServiceRadarr,
	}

	if err := registry.Register(integration); err != nil {
		t.Fatalf("register integration: %v", err)
	}

	got, ok := registry.Get(models.ServiceRadarr)
	if !ok {
		t.Fatal("registered integration not found")
	}

	if got.Type() != models.ServiceRadarr {
		t.Fatalf(
			"unexpected integration type: %s",
			got.Type(),
		)
	}

	if err := registry.Register(integration); err == nil {
		t.Fatal("duplicate registration should fail")
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{
			input: "http://10.0.0.10:7878/",
			want:  "http://10.0.0.10:7878",
		},
		{
			input: "https://radarr.example.com///",
			want:  "https://radarr.example.com",
		},
		{
			input:   "ftp://example.com",
			wantErr: true,
		},
		{
			input:   "",
			wantErr: true,
		},
	}

	for _, test := range tests {
		got, err := NormalizeBaseURL(test.input)

		if test.wantErr {
			if err == nil {
				t.Fatalf(
					"expected error for %q",
					test.input,
				)
			}

			continue
		}

		if err != nil {
			t.Fatalf(
				"unexpected error for %q: %v",
				test.input,
				err,
			)
		}

		if got != test.want {
			t.Fatalf(
				"NormalizeBaseURL(%q) = %q, want %q",
				test.input,
				got,
				test.want,
			)
		}
	}
}
