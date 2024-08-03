package compose

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
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
			wantErr: "port 'target' must be an integer between 1 and 32767",
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
			name:    "Host mode and protocol, published range xfail",
			input:   types.ServicePortConfig{Mode: "host", Target: 1234, Published: "1511-2222"},
			wantErr: "port 'published' range must include 'target': 1511-2222",
		},
		{
			name:    "Host mode and protocol, published range xfail",
			input:   types.ServicePortConfig{Mode: "host", Target: 1234, Published: "22222"},
			wantErr: "port 'published' must be empty or equal to 'target': 22222",
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
			wantErr: "port 'host_ip' is not supported",
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
			fixupPort(&tt.input)
			got := convertPort(tt.input)
			if got.String() != tt.expected.String() {
				t.Errorf("convertPort() got %v, want %v", got, tt.expected.String())
			}
		})
	}
}

func TestComposeBlob(t *testing.T) {
	var compose structpb.Struct
	if err := json.Unmarshal([]byte(`{"name":"test","services":{"test":{"image":"nginx"}}}`), &compose); err != nil {
		t.Fatal(err)
	}

	asmap := compose.AsMap()
	t.Log(asmap)

	p, err := loader.LoadWithContext(context.Background(), types.ConfigDetails{ConfigFiles: []types.ConfigFile{{Config: compose.AsMap()}}}, func(o *loader.Options) {
		o.SetProjectName(compose.Fields["name"].GetStringValue(), true) // HACK: workaround for a bug in compose-go where it insists on loading the project name from the first file
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(p)

	blob, err := proto.Marshal(&compose)
	if err != nil {
		t.Fatal(err)
	}
	if len(blob) != 62 {
		t.Errorf("expected empty blob, got %v", blob)
	}
}
