package compose

import (
	"context"
	"slices"

	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func fixupDeviceCount(count composeTypes.DeviceCount) int {
	if count == -1 {
		return 1
	}
	return int(count)
}

func gpuDeviceCount(service *composeTypes.ServiceConfig) int {
	count := 0
	if service.Deploy != nil &&
		service.Deploy.Resources.Reservations != nil {
		for _, device := range service.Deploy.Resources.Reservations.Devices {
			if slices.Contains(device.Capabilities, "gpu") {
				count += fixupDeviceCount(device.Count)
			}
		}
	}
	return count
}

func GetNumOfGPUs(ctx context.Context, project *composeTypes.Project) int {
	numGPUs := 0
	for _, service := range project.Services {
		numGPUs += gpuDeviceCount(&service)
	}
	return numGPUs
}
