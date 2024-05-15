package client

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

var _ pflag.Value = (*Provider)(nil)

func TestProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		want     Provider
		wantErr  bool
	}{
		{
			name:     "valid provider defang",
			provider: "defang",
			want:     ProviderDefang,
			wantErr:  false,
		},
		{
			name:     "valid provider Defang",
			provider: "Defang",
			want:     ProviderDefang,
			wantErr:  false,
		},
		{
			name:     "invalid provider",
			provider: "invalid",
			wantErr:  true,
		},
		{
			name:     "empty provider",
			provider: "",
			wantErr:  true,
		},
		{
			name:     "valid provider aws",
			provider: "aws",
			want:     ProviderAWS,
			wantErr:  false,
		},
		{
			name:     "valid provider AWS",
			provider: "AWS",
			want:     ProviderAWS,
			wantErr:  false,
		},
		{
			name:     "valid provider auto",
			provider: "auto",
			want:     ProviderAuto,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Provider
			if err := p.Set(tt.provider); (err != nil) != tt.wantErr {
				t.Errorf("Provider.Set() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && p.String() != strings.ToLower(tt.provider) {
				t.Errorf("Provider.String() = %v, want %v", p.String(), tt.provider)
			}
			if p != tt.want {
				t.Errorf("Provider.Set() = %v, want %v", p, tt.want)
			}
		})
	}
}
