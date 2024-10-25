package quota

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/compose-spec/compose-go/v2/types"
)

type ServiceQuotas struct {
	Cpus       float32
	Gpus       uint32
	MemoryMiB  float32
	Replicas   int
	ShmSizeMiB float32
}

func (q ServiceQuotas) Validate(service *types.ServiceConfig) error {
	if service.Name == "" {
		return errors.New("service name is required") // CodeInvalidArgument
	}

	if service.Build != nil {
		if service.Build.Context == "" {
			return errors.New("build.context is required") // CodeInvalidArgument
		}
		if shmSizeMiB := float32(service.Build.ShmSize) / compose.MiB; shmSizeMiB > q.ShmSizeMiB || service.Build.ShmSize < 0 {
			return fmt.Errorf("build.shm_size %v MiB exceeds quota %v MiB", shmSizeMiB, q.ShmSizeMiB) // CodeInvalidArgument
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
		if port.Mode == compose.Mode_INGRESS {
			hasIngress = true
			if port.Protocol == compose.Protocol_UDP {
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
		// Technically this should test for <= but both interval and timeout have 30s as the default value in compose spec
		interval := getOrZero(service.HealthCheck.Interval)
		timeout := getOrZero(service.HealthCheck.Timeout)
		if interval > 0 && interval < timeout {
			return errors.New("invalid healthcheck: timeout must be less than the interval")
		}
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
				return fmt.Errorf("invalid healthcheck: ingress ports require a CMD or CMD-SHELL healthcheck, see https://s.defang.io/healthchecks")
			}
		default:
			return fmt.Errorf("unsupported healthcheck: %v", service.HealthCheck.Test) // this will have been caught by compose-go
		}
	}

	if service.Deploy != nil {
		if service.Deploy.Replicas != nil && *service.Deploy.Replicas > q.Replicas {
			return fmt.Errorf("replicas exceeds quota (max %d)", q.Replicas) // CodeInvalidArgument
		}
		if service.Deploy.Resources.Reservations != nil {
			if float32(service.Deploy.Resources.Reservations.NanoCPUs) > q.Cpus || service.Deploy.Resources.Reservations.NanoCPUs < 0 {
				return fmt.Errorf("cpus exceeds quota (max %v vCPU)", q.Cpus) // CodeInvalidArgument
			}
			if memoryMiB := float32(service.Deploy.Resources.Reservations.MemoryBytes) / compose.MiB; memoryMiB > q.MemoryMiB || service.Deploy.Resources.Reservations.MemoryBytes < 0 {
				return fmt.Errorf("memory %v MiB exceeds quota %v MiB", memoryMiB, q.MemoryMiB) // CodeInvalidArgument
			}
			for _, device := range service.Deploy.Resources.Reservations.Devices {
				if len(device.Capabilities) != 1 || device.Capabilities[0] != "gpu" {
					return errors.New("only GPU devices are supported") // CodeInvalidArgument
				}
				if device.Driver != "" && device.Driver != "nvidia" {
					return errors.New("only nvidia GPU devices are supported") // CodeInvalidArgument
				}
				if q.Gpus == 0 || uint32(device.Count) > q.Gpus {
					return fmt.Errorf("gpu count %v exceeds quota %d", device.Count, q.Gpus) // CodeInvalidArgument
				}
			}
		}
	}

	return nil
}
