package quota

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	compose "github.com/compose-spec/compose-go/v2/types"
)

const Mode_INGRESS = "ingress"
const Mode_HOST = "host"

const Protocol_TCP = "tcp"
const Protocol_UDP = "udp"
const Protocol_HTTP = "http"

const MiB = 1024 * 1024

type Quotas struct {
	Cpus       float32
	Gpus       uint32
	MemoryMiB  float32
	Replicas   uint32
	Services   int
	ShmSizeMiB float32
}

func (q Quotas) Validate(service *compose.ServiceConfig) error {
	if service.Name == "" {
		return errors.New("service name is required") // CodeInvalidArgument
	}

	if service.Build != nil {
		if service.Build.Context == "" {
			return errors.New("build.context is required") // CodeInvalidArgument
		}
		if float32(service.Build.ShmSize)/MiB > q.ShmSizeMiB || service.Build.ShmSize < 0 {
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
		if port.Mode == Mode_INGRESS {
			if port.Protocol == Protocol_TCP || port.Protocol == Protocol_UDP {
				return fmt.Errorf("mode:INGRESS is not supported by protocol:%s", port.Protocol) // CodeInvalidArgument
			}
		}
		if uniquePorts[port.Target] {
			return fmt.Errorf("duplicate port %d", port.Target) // CodeInvalidArgument
		}
		// hasHost = hasHost || port.Mode == v1.Mode_HOST
		hasIngress = hasIngress || port.Mode == Mode_INGRESS
		uniquePorts[port.Target] = true
	}
	if service.HealthCheck != nil && len(service.HealthCheck.Test) > 0 {
		// Technically this should test for <= but both interval and timeout have 30s as the default value in compose spec
		if service.HealthCheck.Interval != nil && *service.HealthCheck.Interval > 0 && *service.HealthCheck.Interval < *service.HealthCheck.Timeout {
			return errors.New("invalid healthcheck: timeout must be less than the interval")
		}
		switch service.HealthCheck.Test[0] {
		case "CMD":
			if hasIngress {
				// For ingress ports, we derive the target group healthcheck path/port from the service healthcheck
				if len(service.HealthCheck.Test) < 3 {
					return errors.New("invalid CMD healthcheck: expected a command and URL")
				}
				if !strings.HasSuffix(service.HealthCheck.Test[1], "curl") && !strings.HasSuffix(service.HealthCheck.Test[1], "wget") {
					return errors.New("invalid CMD healthcheck: expected curl or wget")
				}
				hasHttpUrl := false
				for _, arg := range service.HealthCheck.Test[2:] {
					if u, err := url.Parse(arg); err == nil && u.Scheme == "http" {
						hasHttpUrl = true
						break
					}
				}
				if !hasHttpUrl {
					return errors.New("invalid CMD healthcheck; missing HTTP URL")
				}
			}
		case "NONE":
			if len(service.HealthCheck.Test) != 1 {
				return errors.New("invalid NONE healthcheck; expected no arguments")
			}
			fallthrough // OK iff there are no ingress ports
		case "CMD-SHELL": // OK iff there are no ingress ports; TODO: parse the command and check for curl/wget URL
			if hasIngress {
				return fmt.Errorf("ingress port requires a CMD healthcheck")
			}
		default:
			return fmt.Errorf("unsupported healthcheck: %v", service.HealthCheck.Test)
		}
	}

	if service.Deploy != nil {
		if uint32(getOrZero(service.Deploy.Replicas)) > q.Replicas {
			return fmt.Errorf("replicas exceeds quota (max %d)", q.Replicas) // CodeInvalidArgument
		}
		if service.Deploy.Resources.Reservations != nil {
			if float32(service.Deploy.Resources.Reservations.NanoCPUs) > q.Cpus || service.Deploy.Resources.Reservations.NanoCPUs < 0 {
				return fmt.Errorf("cpus exceeds quota (max %v vCPU)", q.Cpus) // CodeInvalidArgument
			}
			if float32(service.Deploy.Resources.Reservations.MemoryBytes)/MiB > q.MemoryMiB || service.Deploy.Resources.Reservations.MemoryBytes < 0 {
				return fmt.Errorf("memory exceeds quota (max %v MiB)", q.MemoryMiB) // CodeInvalidArgument
			}
			for _, device := range service.Deploy.Resources.Reservations.Devices {
				if len(device.Capabilities) != 1 || device.Capabilities[0] != "gpu" {
					return errors.New("only GPU devices are supported") // CodeInvalidArgument
				}
				if device.Driver != "" && device.Driver != "nvidia" {
					return errors.New("only nvidia GPU devices are supported") // CodeInvalidArgument
				}
				if q.Gpus == 0 || uint32(device.Count) > q.Gpus {
					return fmt.Errorf("gpu count exceeds quota (max %d)", q.Gpus) // CodeInvalidArgument
				}
			}
		}
	}

	return nil
}

func getOrZero[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}
