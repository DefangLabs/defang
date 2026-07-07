package azure

import "testing"

func TestLocationString(t *testing.T) {
	tests := []struct {
		loc  Location
		want string
	}{
		{"", ""},
		{LocationEastUS, "eastus"},
		{LocationWestUS2, "westus2"},
		{LocationWestEurope, "westeurope"},
	}
	for _, tt := range tests {
		if got := tt.loc.String(); got != tt.want {
			t.Errorf("Location(%q).String() = %q, want %q", tt.loc, got, tt.want)
		}
	}
}

func TestLocationPtr(t *testing.T) {
	if p := Location("").Ptr(); p != nil {
		t.Errorf("empty location Ptr() = %v, want nil", p)
	}

	loc := LocationWestUS2
	p := loc.Ptr()
	if p == nil {
		t.Fatalf("Ptr() returned nil for non-empty location")
	}
	if *p != "westus2" {
		t.Errorf("*Ptr() = %q, want %q", *p, "westus2")
	}
}
