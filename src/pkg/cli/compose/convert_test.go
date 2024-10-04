package compose

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/types"
)

func TestConvertPort(t *testing.T) {
	tests := []struct {
		name     string
		input    types.ServicePortConfig
		expected *defangv1.Port
		wantErr  string
	}{
		{
			name:    "No target port xfail",
			input:   types.ServicePortConfig{},
			wantErr: "port 0: 'target' must be an integer between 1 and 32767",
		},
		{
			name:     "Undefined mode and protocol, target only",
			input:    types.ServicePortConfig{Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Undefined mode and protocol, published equals target",
			input:    types.ServicePortConfig{Target: 1234, Published: "1234"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Undefined mode, udp protocol, target only",
			input:    types.ServicePortConfig{Target: 1234, Protocol: "udp"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST, Protocol: defangv1.Protocol_UDP}, // backwards compatibility
		},
		{
			name:     "Undefined mode and published range xfail",
			input:    types.ServicePortConfig{Target: 1234, Published: "1511-2222"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Undefined mode and target in published range xfail",
			input:    types.ServicePortConfig{Target: 1234, Published: "1111-2222"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Undefined mode and published not equals target; common for local development",
			input:    types.ServicePortConfig{Target: 1234, Published: "12345"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Host mode and undefined protocol, target only",
			input:    types.ServicePortConfig{Mode: "host", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST},
		},
		{
			name:     "Host mode and udp protocol, target only",
			input:    types.ServicePortConfig{Mode: "host", Target: 1234, Protocol: "udp"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST, Protocol: defangv1.Protocol_UDP},
		},
		{
			name:     "Host mode and protocol, published equals target",
			input:    types.ServicePortConfig{Mode: "host", Target: 1234, Published: "1234"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST},
		},
		{
			name:     "Host mode and protocol, published range xfail",
			input:    types.ServicePortConfig{Mode: "host", Target: 1234, Published: "1511-2222"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST},
		},
		{
			name:     "Host mode and protocol, published not equals target",
			input:    types.ServicePortConfig{Mode: "host", Target: 1234, Published: "22222"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST},
		},
		{
			name:     "Host mode and protocol, target in published range",
			input:    types.ServicePortConfig{Mode: "host", Target: 1234, Published: "1111-2222"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST},
		},
		{
			name:     "(Implied) ingress mode, defined protocol, only target", // - 1234
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
		{
			name:     "(Implied) ingress mode, udp protocol, only target", // - 1234/udp
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "udp", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST, Protocol: defangv1.Protocol_UDP}, // backwards compatibility
		},
		{
			name:     "(Implied) ingress mode, defined protocol, published equals target", // - 1234:1234
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Published: "1234", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
		{
			name:     "(Implied) ingress mode, udp protocol, published equals target", // - 1234:1234/udp
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "udp", Published: "1234", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST, Protocol: defangv1.Protocol_UDP}, // backwards compatibility
		},
		{
			name:    "Localhost IP, unsupported mode and protocol xfail",
			input:   types.ServicePortConfig{Mode: "ingress", HostIP: "127.0.0.1", Protocol: "tcp", Published: "1234", Target: 1234},
			wantErr: "port 1234: 'host_ip' is not supported",
		},
		{
			name:     "Ingress mode without host IP, single target, published range xfail", // - 1511-2223:1234
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234, Published: "1511-2223"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
		{
			name:     "Ingress mode without host IP, single target, target in published range", // - 1111-2223:1234
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234, Published: "1111-2223"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
		{
			name:     "Ingress mode without host IP, published not equals target; common for local development", // - 12345:1234
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234, Published: "12345"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePort(tt.input)
			if err != nil {
				if tt.wantErr == "" {
					t.Errorf("convertPort() unexpected error: %v", err)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("convertPort() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if tt.wantErr != "" {
				t.Errorf("convertPort() expected error: %v", tt.wantErr)
			}
			got := convertPort(tt.input)
			if got.String() != tt.expected.String() {
				t.Errorf("convertPort() got %v, want %v", got, tt.expected.String())
			}
		})
	}
}

func TestConvert(t *testing.T) {
	testRunCompose(t, func(t *testing.T, path string) {
		loader := NewLoaderWithPath(path)
		proj, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		services, err := ConvertServices(context.Background(), client.MockClient{}, proj.Services, BuildContextIgnore)
		if err != nil {
			t.Fatal(err)
		}

		// The order of the services is not guaranteed, so we sort the services before comparing
		slices.SortFunc(services, func(i, j *defangv1.Service) int { return strings.Compare(i.Name, j.Name) })

		// Convert the protobuf services to pretty JSON for comparison (YAML would include all the zero values)
		actual, err := json.MarshalIndent(services, "", "  ")
		if err != nil {
			t.Fatal(err)
		}

		if err := compare(actual, path+".convert"); err != nil {
			t.Error(err)
		}
	})
}
