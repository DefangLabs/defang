package compose

import (
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/quota"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	compose "github.com/compose-spec/compose-go/v2/types"
)

func getResourceReservations(r compose.Resources) *compose.Resource {
	if r.Reservations == nil {
		// TODO: we might not want to default to all the limits, maybe only memory?
		return r.Limits
	}
	return r.Reservations
}

func convertPlatform(platform string) defangv1.Platform {
	switch strings.ToLower(platform) {
	default:
		warnf("unsupported platform: %q; assuming linux", platform)
		fallthrough
	case "", "linux":
		return defangv1.Platform_LINUX_ANY
	case "linux/amd64", "linux/x86_64": // Docker accepts both
		return defangv1.Platform_LINUX_AMD64
	case "linux/arm64", "linux/arm64/v8", "linux/arm64/v7", "linux/arm64/v6":
		return defangv1.Platform_LINUX_ARM64
	}
}

func network(svccfg *compose.ServiceConfig) defangv1.Network {
	// HACK: Use magic network name "public" to determine if the service is public
	if _, ok := svccfg.Networks["public"]; ok {
		return defangv1.Network_PUBLIC
	}
	// TODO: support external services (w/o LB),
	return defangv1.Network_PRIVATE
}

func fixupPort(port *compose.ServicePortConfig) {
	switch port.Mode {
	case "":
		warnf("No port 'mode' was specified; defaulting to 'ingress' (add 'mode: ingress' to silence)")
		fallthrough
	case "ingress":
		// This code is unnecessarily complex because compose-go silently converts short port: syntax to ingress+tcp
		if port.Protocol != "udp" {
			if port.Published != "" {
				warnf("Published ports are ignored in ingress mode")
			}
			port.Mode = quota.Mode_INGRESS
			if (port.Protocol == "tcp" || port.Protocol == "udp") && port.AppProtocol != "http" {
				warnf("TCP ingress is not supported; assuming HTTP (remove 'protocol' to silence)")
				port.AppProtocol = "http"
			}
			break
		}
		warnf("UDP ports default to 'host' mode (add 'mode: host' to silence)")
		fallthrough
	case "host":
		port.Mode = quota.Mode_HOST
	default:
		panic(fmt.Sprintf("port mode should have been validated to be one of [host ingress] but got: %v", port.Mode))
	}
}

func convertPort(port compose.ServicePortConfig) *defangv1.Port {
	pbPort := &defangv1.Port{
		// Mode      string `yaml:",omitempty" json:"mode,omitempty"`
		// HostIP    string `mapstructure:"host_ip" yaml:"host_ip,omitempty" json:"host_ip,omitempty"`
		// Published string `yaml:",omitempty" json:"published,omitempty"`
		// Protocol  string `yaml:",omitempty" json:"protocol,omitempty"`
		Target: port.Target,
	}

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
	case "ingress":
		pbPort.Mode = defangv1.Mode_INGRESS
	case "host":
		pbPort.Mode = defangv1.Mode_HOST
	default:
		panic(fmt.Sprintf("port mode should have been validated to be one of [host ingress] but got: %q", port.Mode))
	}
	return pbPort
}

func convertPorts(ports []compose.ServicePortConfig) []*defangv1.Port {
	var pbports []*defangv1.Port
	for _, port := range ports {
		pbPort := convertPort(port)
		pbports = append(pbports, pbPort)
	}
	return pbports
}
