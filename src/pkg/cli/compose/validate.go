package compose

import (
	"errors"
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
)

func ValidateService(service *types.ServiceConfig) error {
	if service.Name == "" {
		return errors.New("service name is required") // CodeInvalidArgument
	}

	if service.Build != nil {
		if service.Build.Context == "" {
			return errors.New("build.context is required") // CodeInvalidArgument
		}
	} else {
		if service.Image == "" {
			return errors.New("missing image or build") // CodeInvalidArgument
		}
	}

	// hasHost := false
	hasIngress := false
	uniquePorts := make(map[uint32]bool)
	for _, port := range service.Ports {
		if port.Target < 1 || port.Target > 32767 {
			return fmt.Errorf("port %d is out of range", port.Target) // CodeInvalidArgument
		}
		if port.Mode == Mode_INGRESS {
			hasIngress = true
			if port.Protocol == Protocol_UDP {
				return fmt.Errorf("`mode: ingress` is not supported by `protocol: %s`", port.Protocol) // CodeInvalidArgument
			}
		}
		if uniquePorts[port.Target] {
			return fmt.Errorf("duplicate target port %d", port.Target) // CodeInvalidArgument
		}
		// hasHost = hasHost || port.Mode == v1.Mode_HOST
		uniquePorts[port.Target] = true
	}

	if service.HealthCheck != nil && len(service.HealthCheck.Test) > 0 {
		switch service.HealthCheck.Test[0] {
		case "CMD", "CMD-SHELL":
			if hasIngress {
				// For ingress ports, we derive the target group healthcheck path/port from the service healthcheck
				hasLocalhostUrl := false
				for _, arg := range service.HealthCheck.Test[1:] {
					// Leave the actual parsing to the CD code; here we just check for localhost
					if strings.Contains(arg, "localhost") || strings.Contains(arg, "127.0.0.1") {
						hasLocalhostUrl = true
						break
					}
				}
				if !hasLocalhostUrl {
					return errors.New("invalid healthcheck: ingress ports require an HTTP healthcheck on `localhost`, see https://s.defang.io/healthchecks")
				}
			}
		case "NONE": // OK iff there are no ingress ports
			if hasIngress {
				return errors.New("invalid healthcheck: ingress ports require a CMD or CMD-SHELL healthcheck, see https://s.defang.io/healthchecks")
			}
		default:
			return fmt.Errorf("unsupported healthcheck: %v", service.HealthCheck.Test) // this will have been caught by compose-go
		}
	}

	return nil
}
