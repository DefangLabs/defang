package aci

import (
	"context"
	"fmt"
)

// GetContainerGroupStatus checks the current state of a container group.
// Returns (true, nil) when all containers finished with exit code 0,
// (true, error) when one or more containers failed, and (false, nil) when still running.
func (c *ContainerInstance) GetContainerGroupStatus(ctx context.Context, groupName ContainerGroupName) (bool, error) {
	if groupName == nil {
		return false, nil
	}

	client, err := c.newContainerGroupClient()
	if err != nil {
		return false, err
	}

	resp, err := client.Get(ctx, c.resourceGroupName, *groupName, nil)
	if err != nil {
		return false, fmt.Errorf("failed to get container group: %w", err)
	}

	props := resp.ContainerGroup.Properties
	if props == nil {
		return false, nil
	}

	// Check group-level instance view state
	if props.InstanceView != nil && props.InstanceView.State != nil {
		switch *props.InstanceView.State {
		case "Stopped", "Succeeded":
			// All containers have exited; check exit codes
		case "Failed":
			return true, fmt.Errorf("container group %q failed", *groupName)
		default:
			// Still provisioning or running
			return false, nil
		}
	} else {
		// InstanceView not yet available
		return false, nil
	}

	// Check each container's exit code
	for _, container := range props.Containers {
		if container.Properties == nil || container.Properties.InstanceView == nil {
			continue
		}
		state := container.Properties.InstanceView.CurrentState
		if state == nil {
			continue
		}
		if state.ExitCode != nil && *state.ExitCode != 0 {
			name := "<unknown>"
			if container.Name != nil {
				name = *container.Name
			}
			detail := ""
			if state.DetailStatus != nil {
				detail = ": " + *state.DetailStatus
			}
			return true, fmt.Errorf("container %q exited with code %d%s", name, *state.ExitCode, detail)
		}
	}

	return true, nil
}
