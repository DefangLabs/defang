package client

import "testing"

func TestPinVersion(t *testing.T) {
	tests := []struct {
		name     string
		latest   string
		previous string
		label    string
		upgrade  bool
		want     string
	}{
		{
			name:     "empty previous returns latest",
			latest:   "v2.0",
			previous: "",
			upgrade:  false,
			want:     "v2.0",
		},
		{
			name:     "same version returns latest",
			latest:   "v1.0",
			previous: "v1.0",
			upgrade:  false,
			want:     "v1.0",
		},
		{
			name:     "newer available without upgrade returns previous",
			latest:   "v2.0",
			previous: "v1.0",
			upgrade:  false,
			want:     "v1.0",
		},
		{
			name:     "newer available with upgrade returns latest",
			latest:   "v2.0",
			previous: "v1.0",
			upgrade:  true,
			want:     "v2.0",
		},
		{
			name:     "downgrade without upgrade returns previous",
			latest:   "v1.0",
			previous: "v2.0",
			upgrade:  false,
			want:     "v2.0",
		},
		{
			name:     "downgrade with upgrade returns latest",
			latest:   "v1.0",
			previous: "v2.0",
			upgrade:  true,
			want:     "v1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pinVersion(tt.latest, tt.previous, "test", tt.upgrade)
			if got != tt.want {
				t.Errorf("pinVersion(%q, %q, upgrade=%v) = %q, want %q", tt.latest, tt.previous, tt.upgrade, got, tt.want)
			}
		})
	}
}
