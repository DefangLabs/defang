package compose

import (
	"context"
	"slices"

	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func GetNumOfGPUs(ctx context.Context, project *composeTypes.Project) int {
	numGPUs := 0
	for _, service := range project.Services {
		if service.Deploy != nil &&
			service.Deploy.Resources.Reservations != nil {
			for _, device := range service.Deploy.Resources.Reservations.Devices {
				if slices.Contains(device.Capabilities, "gpu") {
					numGPUs += int(device.Count)
				}
			}
		}
	}
	return numGPUs
}
