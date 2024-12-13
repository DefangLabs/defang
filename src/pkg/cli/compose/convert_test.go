package compose

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func TestConvertPort(t *testing.T) {
	tests := []struct {
		name     string
		input    composeTypes.ServicePortConfig
		expected *defangv1.Port
		wantErr  string
	}{
		{
			name:    "No target port xfail",
			input:   composeTypes.ServicePortConfig{},
			wantErr: "port 0: 'target' must be an integer between 1 and 32767",
		},
		{
			name:     "Undefined mode and protocol, target only",
			input:    composeTypes.ServicePortConfig{Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Undefined mode and protocol, published equals target",
			input:    composeTypes.ServicePortConfig{Target: 1234, Published: "1234"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Undefined mode, udp protocol, target only",
			input:    composeTypes.ServicePortConfig{Target: 1234, Protocol: "udp"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST, Protocol: defangv1.Protocol_UDP}, // backwards compatibility
		},
		{
			name:     "Undefined mode and published range xfail",
			input:    composeTypes.ServicePortConfig{Target: 1234, Published: "1511-2222"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Undefined mode and target in published range xfail",
			input:    composeTypes.ServicePortConfig{Target: 1234, Published: "1111-2222"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Undefined mode and published not equals target; common for local development",
			input:    composeTypes.ServicePortConfig{Target: 1234, Published: "12345"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Host mode and undefined protocol, target only",
			input:    composeTypes.ServicePortConfig{Mode: "host", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST},
		},
		{
			name:     "Host mode and udp protocol, target only",
			input:    composeTypes.ServicePortConfig{Mode: "host", Target: 1234, Protocol: "udp"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST, Protocol: defangv1.Protocol_UDP},
		},
		{
			name:     "Host mode and protocol, published equals target",
			input:    composeTypes.ServicePortConfig{Mode: "host", Target: 1234, Published: "1234"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST},
		},
		{
			name:     "Host mode and protocol, published range xfail",
			input:    composeTypes.ServicePortConfig{Mode: "host", Target: 1234, Published: "1511-2222"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST},
		},
		{
			name:     "Host mode and protocol, published not equals target",
			input:    composeTypes.ServicePortConfig{Mode: "host", Target: 1234, Published: "22222"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST},
		},
		{
			name:     "Host mode and protocol, target in published range",
			input:    composeTypes.ServicePortConfig{Mode: "host", Target: 1234, Published: "1111-2222"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST},
		},
		{
			name:     "(Implied) ingress mode, defined protocol, only target", // - 1234
			input:    composeTypes.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
		{
			name:     "(Implied) ingress mode, udp protocol, only target", // - 1234/udp
			input:    composeTypes.ServicePortConfig{Mode: "ingress", Protocol: "udp", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST, Protocol: defangv1.Protocol_UDP}, // backwards compatibility
		},
		{
			name:     "(Implied) ingress mode, defined protocol, published equals target", // - 1234:1234
			input:    composeTypes.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Published: "1234", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
		{
			name:     "(Implied) ingress mode, udp protocol, published equals target", // - 1234:1234/udp
			input:    composeTypes.ServicePortConfig{Mode: "ingress", Protocol: "udp", Published: "1234", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST, Protocol: defangv1.Protocol_UDP}, // backwards compatibility
		},
		{
			name:    "Localhost IP, unsupported mode and protocol xfail",
			input:   composeTypes.ServicePortConfig{Mode: "ingress", HostIP: "127.0.0.1", Protocol: "tcp", Published: "1234", Target: 1234},
			wantErr: "port 1234: 'host_ip' is not supported",
		},
		{
			name:     "Ingress mode without host IP, single target, published range xfail", // - 1511-2223:1234
			input:    composeTypes.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234, Published: "1511-2223"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
		{
			name:     "Ingress mode without host IP, single target, target in published range", // - 1111-2223:1234
			input:    composeTypes.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234, Published: "1111-2223"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
		{
			name:     "Ingress mode without host IP, published not equals target; common for local development", // - 12345:1234
			input:    composeTypes.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234, Published: "12345"},
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

// TODO: remove this (and change the test cases to avoid using the protobuf)
func convertPort(port composeTypes.ServicePortConfig) *defangv1.Port {
	pbPort := &defangv1.Port{
		// Mode      string `yaml:",omitempty" json:"mode,omitempty"`
		// HostIP    string `mapstructure:"host_ip" yaml:"host_ip,omitempty" json:"host_ip,omitempty"`
		// Published string `yaml:",omitempty" json:"published,omitempty"`
		// Protocol  string `yaml:",omitempty" json:"protocol,omitempty"`
		Target: port.Target,
	}

	// TODO: Use AppProtocol as hint for application protocol
	// https://github.com/compose-spec/compose-spec/blob/main/05-services.md#long-syntax-3
	switch port.Protocol {
	case "":
		pbPort.Protocol = defangv1.Protocol_ANY // defaults to HTTP in CD
	case "tcp":
		pbPort.Protocol = defangv1.Protocol_TCP
	case "udp":
		pbPort.Protocol = defangv1.Protocol_UDP
	case "http": // TODO: not per spec; should use AppProtocol
		pbPort.Protocol = defangv1.Protocol_HTTP
	case "http2": // TODO: not per spec; should use AppProtocol
		pbPort.Protocol = defangv1.Protocol_HTTP2
	case "grpc": // TODO: not per spec; should use AppProtocol
		pbPort.Protocol = defangv1.Protocol_GRPC
	default:
		panic(fmt.Sprintf("port 'protocol' should have been validated to be one of [tcp udp http http2 grpc] but got: %q", port.Protocol))
	}

	switch port.AppProtocol {
	case "http":
		pbPort.Protocol = defangv1.Protocol_HTTP
	case "http2":
		pbPort.Protocol = defangv1.Protocol_HTTP2
	case "grpc":
		pbPort.Protocol = defangv1.Protocol_GRPC
	}

	switch port.Mode {
	case "":
		// TODO: This never happens now as compose-go set default to "ingress"
		term.Warnf("No port 'mode' was specified; defaulting to 'ingress' (add 'mode: ingress' to silence)")
		fallthrough
	case "ingress":
		// This code is unnecessarily complex because compose-go silently converts short port: syntax to ingress+tcp
		if port.Protocol != "udp" {
			if port.Published != "" {
				term.Warnf("Published ports are ignored in ingress mode")
			}
			pbPort.Mode = defangv1.Mode_INGRESS
			if pbPort.Protocol == defangv1.Protocol_TCP {
				pbPort.Protocol = defangv1.Protocol_HTTP
			}
			break
		}
		term.Warnf("UDP ports default to 'host' mode (add 'mode: host' to silence)")
		fallthrough
	case "host":
		pbPort.Mode = defangv1.Mode_HOST
	default:
		panic(fmt.Sprintf("port mode should have been validated to be one of [host ingress] but got: %q", port.Mode))
	}
	return pbPort
}
