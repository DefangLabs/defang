package aws

import (
	"errors"
	"fmt"
	"slices"

	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

// Defang supports a subset of instances, max of 8 GPUs
// https://aws.amazon.com/ec2/instance-types/#Accelerated_Computing
const MAX_GPUS = 8

var ErrGPUQuotaExceeded = fmt.Errorf("GPU quota exceeded, max is %d", MAX_GPUS)
var ErrZeroGPUsRequested = errors.New("zero GPUs requested")

func ValidateGPUResources(service composeTypes.ServiceConfig) error {
	if service.Deploy != nil &&
		service.Deploy.Resources.Reservations != nil {
		for _, device := range service.Deploy.Resources.Reservations.Devices {
			if slices.Contains(device.Capabilities, "gpu") {

				if device.Count > MAX_GPUS {
					return ErrGPUQuotaExceeded
				}

				if device.Count < 1 {
					return ErrZeroGPUsRequested
				}
			}
		}
	}
	return nil
}
