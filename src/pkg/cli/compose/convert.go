package compose

import (
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/term"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func getResourceReservations(r composeTypes.Resources) *composeTypes.Resource {
	if r.Reservations == nil {
		// TODO: we might not want to default to all the limits, maybe only memory?
		return r.Limits
	}
	return r.Reservations
}

func fixupPort(port *composeTypes.ServicePortConfig) {
	switch port.Mode {
	case "":
		term.Warnf("port %d: no 'mode' was specified; defaulting to 'ingress' (add 'mode: ingress' to silence)", port.Target)
		fallthrough
	case Mode_INGRESS:
		// This code is unnecessarily complex because compose-go silently converts short `ports:` syntax to ingress+tcp
		if port.Protocol != Protocol_UDP {
			if port.Published != "" {
				term.Debugf("port %d: ignoring 'published: %s' in 'ingress' mode", port.Target, port.Published)
			}
			if (port.Protocol == Protocol_TCP || port.Protocol == Protocol_UDP) && port.AppProtocol != "http" {
				// TCP ingress is not supported; assuming HTTP (remove 'protocol' to silence)"
				port.AppProtocol = "http"
			}
			break
		}
		term.Warnf("port %d: UDP ports default to 'host' mode (add 'mode: host' to silence)", port.Target)
		port.Mode = Mode_HOST
	case Mode_HOST:
		// no-op
	default:
		panic(fmt.Sprintf("port %d: 'mode' should have been validated to be one of [host ingress] but got: %v", port.Target, port.Mode))
	}
}
