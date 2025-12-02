package modes

import (
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
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
			want:  Mode(defangv1.DeploymentMode_DEVELOPMENT),
		},
		{
			name:  "balanced",
			input: "BALANCED",
			want:  Mode(defangv1.DeploymentMode_STAGING),
		},
		{
			name:  "high_availability",
			input: "HIGH_AVAILABILITY",
			want:  Mode(defangv1.DeploymentMode_PRODUCTION),
		},
		{
			name:    "invalid",
			input:   "INVALID",
			wantErr: true,
		},
		{
			name:    "development (deprecated)",
			input:   "DEVELOPMENT",
			want:    Mode(defangv1.DeploymentMode_DEVELOPMENT),
			wantErr: false,
		},
		{
			name:    "staging (deprecated)",
			input:   "STAGING",
			want:    Mode(defangv1.DeploymentMode_STAGING),
			wantErr: false,
		},
		{
			name:    "production (deprecated)",
			input:   "PRODUCTION",
			want:    Mode(defangv1.DeploymentMode_PRODUCTION),
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
			mode: Mode(defangv1.DeploymentMode_DEVELOPMENT),
			want: "AFFORDABLE",
		},
		{
			name: "balanced",
			mode: Mode(defangv1.DeploymentMode_STAGING),
			want: "BALANCED",
		},
		{
			name: "high_availability",
			mode: Mode(defangv1.DeploymentMode_PRODUCTION),
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
