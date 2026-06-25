package modes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMode_Set(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Mode
		wantErr bool
	}{
		{
			name:  "affordable",
			input: "AFFORDABLE",
			want:  ModeAffordable,
		},
		{
			name:  "balanced",
			input: "BALANCED",
			want:  ModeBalanced,
		},
		{
			name:  "high_availability",
			input: "HIGH_AVAILABILITY",
			want:  ModeHighAvailability,
		},
		{
			name:    "development (deprecated)",
			input:   "DEVELOPMENT",
			want:    ModeAffordable,
			wantErr: false,
		},
		{
			name:    "staging (deprecated)",
			input:   "STAGING",
			want:    ModeBalanced,
			wantErr: false,
		},
		{
			name:    "production (deprecated)",
			input:   "PRODUCTION",
			want:    ModeHighAvailability,
			wantErr: false,
		},
		{
			name:    "unspecified",
			input:   "",
			want:    ModeUnspecified,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m Mode
			err := m.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Mode.Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				assert.Equal(t, tt.want, m)
			}
		})
	}
}

func TestMode_String(t *testing.T) {
	tests := []struct {
		name string
		mode Mode
		want string
	}{
		{
			name: "affordable",
			mode: ModeAffordable,
			want: "AFFORDABLE",
		},
		{
			name: "balanced",
			mode: ModeBalanced,
			want: "BALANCED",
		},
		{
			name: "high_availability",
			mode: ModeHighAvailability,
			want: "HIGH_AVAILABILITY",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("Mode.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllDeploymentModes(t *testing.T) {
	want := []string{"AFFORDABLE", "BALANCED", "HIGH_AVAILABILITY"}
	if got := AllDeploymentModes(); !assert.Equal(t, want, got) {
		t.Errorf("AllDeploymentModes() = %v, want %v", got, want)
	}
}
