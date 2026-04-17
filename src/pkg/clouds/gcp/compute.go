package gcp

import (
	"context"
	"fmt"

	compute "google.golang.org/api/compute/v1"
)

// GetInstanceGroupManagerLabels fetches the allInstancesConfig.properties.labels from a regional
// instance group manager. The patch audit log only carries changed fields (e.g. the new instance
// template version), so the defang-service label is absent from the audit log request body and
// must be read from the live resource.
func (gcp Gcp) GetInstanceGroupManagerLabels(ctx context.Context, project, region, name string) (map[string]string, error) {
	svc, err := compute.NewService(ctx, gcp.Options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client: %w", err)
	}
	mgr, err := svc.RegionInstanceGroupManagers.Get(project, region, name).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get instance group manager %q: %w", name, err)
	}
	if mgr.AllInstancesConfig == nil || mgr.AllInstancesConfig.Properties == nil {
		return nil, nil
	}
	return mgr.AllInstancesConfig.Properties.Labels, nil
}
