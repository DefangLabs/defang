package quota

import (
	"errors"
	"fmt"
	"regexp"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Quotas struct {
	Cpus       float32
	Gpus       uint32
	MemoryMiB  float32
	Replicas   uint32
	Services   int
	ShmSizeMiB float32
}

// Based on https://www.ietf.org/rfc/rfc3986.txt, using pattern for query (which is a superset of path's pchar) but removing the single quote
//
//	query       = *( pchar / "/" / "?" )
//	pchar         = unreserved / pct-encoded / sub-delims / ":" / "@"
//	unreserved    = ALPHA / DIGIT / "-" / "." / "_" / "~"
//	pct-encoded   = "%" HEXDIG HEXDIG
//	sub-delims    = "!" / "$" / "&" / "'" / "(" / ")" / "*" / "+" / "," / ";" / "="
var healthcheckUrlRegex = regexp.MustCompile(`(?i)(http://)?(localhost|127.0.0.1)(:\d{1,5})?(/(?:[?a-z0-9._~!$&()*+,;=:@-]|%[a-f0-9]{2})*)*`)

func (q Quotas) Validate(service *defangv1.Service) error {
	if service.Name == "" {
		return errors.New("service name is required") // CodeInvalidArgument
	}

	if service.Build != nil {
		if service.Build.Context == "" {
			return errors.New("build.context is required") // CodeInvalidArgument
		}
		if service.Build.ShmSize > q.ShmSizeMiB || service.Build.ShmSize < 0 {
			return fmt.Errorf("build.shm_size exceeds quota (max %v MiB)", q.ShmSizeMiB) // CodeInvalidArgument
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
		if port.Mode == defangv1.Mode_INGRESS {
			if port.Protocol == defangv1.Protocol_TCP || port.Protocol == defangv1.Protocol_UDP {
				return fmt.Errorf("mode:INGRESS is not supported by protocol:%s", port.Protocol) // CodeInvalidArgument
			}
		}
		if uniquePorts[port.Target] {
			return fmt.Errorf("duplicate port %d", port.Target) // CodeInvalidArgument
		}
		// hasHost = hasHost || port.Mode == v1.Mode_HOST
		hasIngress = hasIngress || port.Mode == defangv1.Mode_INGRESS
		uniquePorts[port.Target] = true
	}
	if service.Healthcheck != nil && len(service.Healthcheck.Test) > 0 {
		// Technically this should test for <= but both interval and timeout have 30s as the default value in compose spec
		if service.Healthcheck.Interval > 0 && service.Healthcheck.Interval < service.Healthcheck.Timeout {
			return errors.New("invalid healthcheck: timeout must be less than the interval")
		}
		switch service.Healthcheck.Test[0] {
		case "CMD":
			if hasIngress {
				// For ingress ports, we derive the target group healthcheck path/port from the service healthcheck
				if len(service.Healthcheck.Test) < 3 {
					return errors.New("invalid CMD healthcheck: expected a command and URL")
				}
				hasHttpUrl := false
				for _, arg := range service.Healthcheck.Test[2:] {
					if healthcheckUrlRegex.MatchString(arg) {
						hasHttpUrl = true
						break
					}
				}
				if !hasHttpUrl {
					return errors.New("invalid CMD healthcheck; missing HTTP URL")
				}
			}
		case "NONE":
			if len(service.Healthcheck.Test) != 1 {
				return errors.New("invalid NONE healthcheck; expected no arguments")
			}
			fallthrough // OK iff there are no ingress ports
		case "CMD-SHELL": // OK iff there are no ingress ports; TODO: parse the command and check for curl/wget URL
			if hasIngress {
				return fmt.Errorf("ingress port requires a CMD healthcheck")
			}
		default:
			return fmt.Errorf("unsupported healthcheck: %v", service.Healthcheck.Test)
		}
	}

	if service.Deploy != nil {
		if service.Deploy.Replicas > q.Replicas {
			return fmt.Errorf("replicas exceeds quota (max %d)", q.Replicas) // CodeInvalidArgument
		}
		if service.Deploy.Resources != nil && service.Deploy.Resources.Reservations != nil {
			if service.Deploy.Resources.Reservations.Cpus > q.Cpus || service.Deploy.Resources.Reservations.Cpus < 0 {
				return fmt.Errorf("cpus exceeds quota (max %v vCPU)", q.Cpus) // CodeInvalidArgument
			}
			if service.Deploy.Resources.Reservations.Memory > q.MemoryMiB || service.Deploy.Resources.Reservations.Memory < 0 {
				return fmt.Errorf("memory exceeds quota (max %v MiB)", q.MemoryMiB) // CodeInvalidArgument
			}
			for _, device := range service.Deploy.Resources.Reservations.Devices {
				if len(device.Capabilities) != 1 || device.Capabilities[0] != "gpu" {
					return errors.New("only GPU devices are supported") // CodeInvalidArgument
				}
				if device.Driver != "" && device.Driver != "nvidia" {
					return errors.New("only nvidia GPU devices are supported") // CodeInvalidArgument
				}
				if q.Gpus == 0 || device.Count > q.Gpus {
					return fmt.Errorf("gpu count exceeds quota (max %d)", q.Gpus) // CodeInvalidArgument
				}
			}
		}
	}

	return nil
}
