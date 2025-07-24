package quota

import (
	"errors"
	"fmt"

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

func (q ServiceQuotas) ValidateQuotas(service *types.ServiceConfig) error {
	if service.Build != nil {
		if shmSizeMiB := float32(service.Build.ShmSize) / compose.MiB; shmSizeMiB > q.ShmSizeMiB || service.Build.ShmSize < 0 {
			return fmt.Errorf("build.shm_size %v MiB exceeds quota %v MiB", shmSizeMiB, q.ShmSizeMiB) // CodeInvalidArgument
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
				var deviceCount uint32
				if device.Count == -1 {
					deviceCount = 1
				} else {
					// #nosec G115 - device.Count is expected to be a small number
					deviceCount = uint32(device.Count)
				}
				if q.Gpus == 0 || deviceCount > q.Gpus {
					return fmt.Errorf("gpu count %v exceeds quota %d", device.Count, q.Gpus) // CodeInvalidArgument
				}
			}
		}
	}

	return nil
}
