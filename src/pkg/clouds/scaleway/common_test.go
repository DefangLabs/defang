package scaleway

import (
	"testing"
)

func TestDefaultZone(t *testing.T) {
	tests := []struct {
		region string
		want   string
	}{
		{"fr-par", "fr-par-1"},
		{"nl-ams", "nl-ams-1"},
		{"pl-waw", "pl-waw-1"},
	}
	for _, tt := range tests {
		t.Run(tt.region, func(t *testing.T) {
			if got := DefaultZone(tt.region); got != tt.want {
				t.Errorf("DefaultZone(%q) = %q, want %q", tt.region, got, tt.want)
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient("SCWAK", "secret", "proj-123", "fr-par")
	if c.AccessKey != "SCWAK" {
		t.Errorf("AccessKey = %q, want %q", c.AccessKey, "SCWAK")
	}
	if c.SecretKey != "secret" {
		t.Errorf("SecretKey = %q, want %q", c.SecretKey, "secret")
	}
	if c.ProjectID != "proj-123" {
		t.Errorf("ProjectID = %q, want %q", c.ProjectID, "proj-123")
	}
	if c.Region != "fr-par" {
		t.Errorf("Region = %q, want %q", c.Region, "fr-par")
	}
	if c.HTTPClient == nil {
		t.Error("HTTPClient should not be nil")
	}
}

func TestRegionURL(t *testing.T) {
	c := NewClient("ak", "sk", "proj", "fr-par")
	got := c.regionURL("secret-manager", "v1beta1")
	want := "https://api.scaleway.com/secret-manager/v1beta1/regions/fr-par"
	if got != want {
		t.Errorf("regionURL() = %q, want %q", got, want)
	}
}
