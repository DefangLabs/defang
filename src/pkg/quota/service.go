package quota

import (
	"errors"
	"fmt"
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ServiceQuotas struct {
	Cpus       float32
	Gpus       uint32
	MemoryMiB  float32
	Replicas   uint32
	ShmSizeMiB float32
}

func (q ServiceQuotas) Validate(service *defangv1.Service) error {
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
			hasIngress = true
			if port.Protocol == defangv1.Protocol_TCP || port.Protocol == defangv1.Protocol_UDP {
				return fmt.Errorf("mode:INGRESS is not supported by protocol:%s", port.Protocol) // CodeInvalidArgument
			}
		}
		if uniquePorts[port.Target] {
			return fmt.Errorf("duplicate target port %d", port.Target) // CodeInvalidArgument
		}
		// hasHost = hasHost || port.Mode == v1.Mode_HOST
		uniquePorts[port.Target] = true
	}
	if service.Healthcheck != nil && len(service.Healthcheck.Test) > 0 {
		// Technically this should test for <= but both interval and timeout have 30s as the default value in compose spec
		if service.Healthcheck.Interval > 0 && service.Healthcheck.Interval < service.Healthcheck.Timeout {
			return errors.New("invalid healthcheck: timeout must be less than the interval")
		}
		switch service.Healthcheck.Test[0] {
		case "CMD", "CMD-SHELL":
			if hasIngress {
				// For ingress ports, we derive the target group healthcheck path/port from the service healthcheck
				hasLocalhostUrl := false
				for _, arg := range service.Healthcheck.Test[1:] {
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
			return fmt.Errorf("unsupported healthcheck: %v", service.Healthcheck.Test) // this will have been caught by compose-go
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
