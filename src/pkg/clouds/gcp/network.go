package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/googleapi"

	compute "google.golang.org/api/compute/v1"
)

// ErrResourceInUse is returned when GCP refuses a delete because something still references the
// resource. For subnetworks this is usually not a real blocker but a timing one: Cloud Run holds a
// subnet's IP addresses for 1-2 hours after the last service using it is deleted.
// See https://docs.cloud.google.com/run/docs/configuring/vpc-direct-vpc
var ErrResourceInUse = errors.New("resource still in use")

// networkService builds a compute client. The compute v1 REST API is used rather than the newer
// apiv1 gRPC clients because it is already a dependency (see GetInstanceGroupManagerLabels) and
// covers every call needed here, including networks.removePeering.
func (gcp Gcp) networkService(ctx context.Context) (*compute.Service, error) {
	svc, err := compute.NewService(ctx, gcp.Options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client: %w", err)
	}
	return svc, nil
}

// ResourceName returns the last path segment of a GCP self-link, which is the resource's name.
// Self-links are how the compute API cross-references resources, but deletes take a bare name.
func ResourceName(selfLink string) string {
	return selfLink[strings.LastIndex(selfLink, "/")+1:]
}

// sameResource compares two self-links, or a self-link and a bare name, for the same resource.
// The API is inconsistent about which form it returns (a network interface may carry a full
// https:// self-link while a list response carries a /projects/... path), so compare by name.
func sameResource(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return ResourceName(a) == ResourceName(b)
}

// annotateOperationError converts a compute API error into ErrResourceInUse when GCP reported
// that the resource is still referenced, so callers can tell a timing window from a real failure.
func annotateOperationError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		for _, e := range apiErr.Errors {
			if e.Reason == "resourceInUseByAnotherResource" {
				return fmt.Errorf("%w: %s", ErrResourceInUse, e.Message)
			}
		}
	}
	return err
}

// waitForGlobalOperation blocks until a global operation finishes and reports its error, if any.
// A compute delete is asynchronous: the call returns an Operation and the interesting failure
// (for example a subnetwork still holding IP addresses) only appears once it completes.
func (gcp Gcp) waitForGlobalOperation(ctx context.Context, svc *compute.Service, op *compute.Operation) error {
	for op != nil && op.Status != "DONE" {
		done, err := svc.GlobalOperations.Wait(gcp.ProjectId, op.Name).Context(ctx).Do()
		if err != nil {
			return annotateOperationError(err)
		}
		op = done
	}
	return operationError(op)
}

func (gcp Gcp) waitForRegionOperation(ctx context.Context, svc *compute.Service, region string, op *compute.Operation) error {
	for op != nil && op.Status != "DONE" {
		done, err := svc.RegionOperations.Wait(gcp.ProjectId, region, op.Name).Context(ctx).Do()
		if err != nil {
			return annotateOperationError(err)
		}
		op = done
	}
	return operationError(op)
}

// operationError turns a finished operation's error payload into a Go error. The operation itself
// succeeds as an API call even when the work it describes failed, so this must be checked.
func operationError(op *compute.Operation) error {
	if op == nil || op.Error == nil || len(op.Error.Errors) == 0 {
		return nil
	}
	e := op.Error.Errors[0]
	err := fmt.Errorf("%s: %s", e.Code, e.Message)
	if e.Code == "RESOURCE_IN_USE_BY_ANOTHER_RESOURCE" {
		return fmt.Errorf("%w: %s", ErrResourceInUse, e.Message)
	}
	return err
}

// ListNetworksByPrefix returns the VPC networks in the project whose name starts with prefix.
// Networks do not support labels in GCP, so the deterministic name is the only way to find them.
func (gcp Gcp) ListNetworksByPrefix(ctx context.Context, prefix string) ([]*compute.Network, error) {
	svc, err := gcp.networkService(ctx)
	if err != nil {
		return nil, err
	}
	var networks []*compute.Network
	err = svc.Networks.List(gcp.ProjectId).Pages(ctx, func(page *compute.NetworkList) error {
		for _, n := range page.Items {
			if strings.HasPrefix(n.Name, prefix) {
				networks = append(networks, n)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}
	return networks, nil
}

// NetworkUsage names the live resources still attached to a network. A network with any usage is
// in service and must not be cleaned up: the leftovers of a torn-down stack have none.
type NetworkUsage struct {
	Instances       []string
	ForwardingRules []string
	CloudRun        []string
}

func (u NetworkUsage) InUse() bool {
	return len(u.Instances) > 0 || len(u.ForwardingRules) > 0 || len(u.CloudRun) > 0
}

// Describe renders the usage for a human, so a skipped network says what is holding it.
func (u NetworkUsage) Describe() string {
	var parts []string
	if n := len(u.Instances); n > 0 {
		parts = append(parts, fmt.Sprintf("%d instance(s): %s", n, strings.Join(u.Instances, ", ")))
	}
	if n := len(u.ForwardingRules); n > 0 {
		parts = append(parts, fmt.Sprintf("%d forwarding rule(s): %s", n, strings.Join(u.ForwardingRules, ", ")))
	}
	if n := len(u.CloudRun); n > 0 {
		parts = append(parts, fmt.Sprintf("%d Cloud Run service(s): %s", n, strings.Join(u.CloudRun, ", ")))
	}
	return strings.Join(parts, "; ")
}

// GetNetworkUsage reports the compute resources still attached to the given network. Cloud Run
// services are counted separately by GetCloudRunNetworkUsage, because they are not compute
// resources and need the Cloud Run API.
func (gcp Gcp) GetNetworkUsage(ctx context.Context, networkSelfLink string) (NetworkUsage, error) {
	var usage NetworkUsage
	svc, err := gcp.networkService(ctx)
	if err != nil {
		return usage, err
	}

	err = svc.Instances.AggregatedList(gcp.ProjectId).Pages(ctx, func(page *compute.InstanceAggregatedList) error {
		for _, scoped := range page.Items {
			for _, inst := range scoped.Instances {
				for _, nic := range inst.NetworkInterfaces {
					if sameResource(nic.Network, networkSelfLink) {
						usage.Instances = append(usage.Instances, inst.Name)
						break
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return usage, fmt.Errorf("failed to list instances: %w", err)
	}

	err = svc.ForwardingRules.AggregatedList(gcp.ProjectId).Pages(ctx, func(page *compute.ForwardingRuleAggregatedList) error {
		for _, scoped := range page.Items {
			for _, fr := range scoped.ForwardingRules {
				if sameResource(fr.Network, networkSelfLink) {
					usage.ForwardingRules = append(usage.ForwardingRules, fr.Name)
				}
			}
		}
		return nil
	})
	if err != nil {
		return usage, fmt.Errorf("failed to list forwarding rules: %w", err)
	}

	return usage, nil
}

// ListSubnetworks returns the subnetworks attached to a network, in the network's own region set.
// The network resource already carries their self-links, so no aggregated list is needed.
func (gcp Gcp) ListSubnetworks(ctx context.Context, network *compute.Network) ([]*compute.Subnetwork, error) {
	svc, err := gcp.networkService(ctx)
	if err != nil {
		return nil, err
	}
	var subnets []*compute.Subnetwork
	for _, link := range network.Subnetworks {
		region := subnetworkRegion(link)
		if region == "" {
			return nil, fmt.Errorf("cannot determine region of subnetwork %q", link)
		}
		sub, err := svc.Subnetworks.Get(gcp.ProjectId, region, ResourceName(link)).Context(ctx).Do()
		if err != nil {
			if isNotFound(err) {
				continue // already gone
			}
			return nil, fmt.Errorf("failed to get subnetwork %q: %w", link, err)
		}
		subnets = append(subnets, sub)
	}
	return subnets, nil
}

// subnetworkRegion extracts the region from a subnetwork self-link, which has the shape
// .../projects/<project>/regions/<region>/subnetworks/<name>. Subnetwork deletes are regional,
// and the region is not a separate field on the link.
func subnetworkRegion(selfLink string) string {
	parts := strings.Split(selfLink, "/")
	for i, p := range parts {
		if p == "regions" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// ListInstanceTemplatesForNetwork returns the instance templates whose network interfaces
// reference the given network. A template holds the subnet it points at, so GCP refuses to delete
// the subnet while one exists.
func (gcp Gcp) ListInstanceTemplatesForNetwork(ctx context.Context, networkSelfLink string) ([]*compute.InstanceTemplate, error) {
	svc, err := gcp.networkService(ctx)
	if err != nil {
		return nil, err
	}
	var templates []*compute.InstanceTemplate
	err = svc.InstanceTemplates.List(gcp.ProjectId).Pages(ctx, func(page *compute.InstanceTemplateList) error {
		for _, tmpl := range page.Items {
			if tmpl.Properties == nil {
				continue
			}
			for _, nic := range tmpl.Properties.NetworkInterfaces {
				if sameResource(nic.Network, networkSelfLink) {
					templates = append(templates, tmpl)
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list instance templates: %w", err)
	}
	return templates, nil
}

// ListPeeringRangesForNetwork returns the internal global addresses reserved for VPC peering on
// the given network. The peering holds the range, so the range can only go after removePeering.
func (gcp Gcp) ListPeeringRangesForNetwork(ctx context.Context, networkSelfLink string) ([]*compute.Address, error) {
	svc, err := gcp.networkService(ctx)
	if err != nil {
		return nil, err
	}
	var addresses []*compute.Address
	err = svc.GlobalAddresses.List(gcp.ProjectId).Pages(ctx, func(page *compute.AddressList) error {
		for _, addr := range page.Items {
			if addr.Purpose == "VPC_PEERING" && sameResource(addr.Network, networkSelfLink) {
				addresses = append(addresses, addr)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list global addresses: %w", err)
	}
	return addresses, nil
}

// RemoveNetworkPeering removes a peering from a network through the Compute API. This is the call
// the GCP console uses, and the only one that works: the servicenetworking deleteConnection API
// refuses while a producer service exists, and can fail even after the producers are gone
// (hashicorp/terraform-provider-google#18834). Removing the peering is safe here only because it
// is done on the way to deleting the network itself.
func (gcp Gcp) RemoveNetworkPeering(ctx context.Context, networkName, peeringName string) error {
	svc, err := gcp.networkService(ctx)
	if err != nil {
		return err
	}
	op, err := svc.Networks.RemovePeering(gcp.ProjectId, networkName, &compute.NetworksRemovePeeringRequest{
		Name: peeringName,
	}).Context(ctx).Do()
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return annotateOperationError(err)
	}
	return gcp.waitForGlobalOperation(ctx, svc, op)
}

func (gcp Gcp) DeleteInstanceTemplate(ctx context.Context, name string) error {
	svc, err := gcp.networkService(ctx)
	if err != nil {
		return err
	}
	op, err := svc.InstanceTemplates.Delete(gcp.ProjectId, name).Context(ctx).Do()
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return annotateOperationError(err)
	}
	return gcp.waitForGlobalOperation(ctx, svc, op)
}

func (gcp Gcp) DeleteGlobalAddress(ctx context.Context, name string) error {
	svc, err := gcp.networkService(ctx)
	if err != nil {
		return err
	}
	op, err := svc.GlobalAddresses.Delete(gcp.ProjectId, name).Context(ctx).Do()
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return annotateOperationError(err)
	}
	return gcp.waitForGlobalOperation(ctx, svc, op)
}

func (gcp Gcp) DeleteSubnetwork(ctx context.Context, region, name string) error {
	svc, err := gcp.networkService(ctx)
	if err != nil {
		return err
	}
	op, err := svc.Subnetworks.Delete(gcp.ProjectId, region, name).Context(ctx).Do()
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return annotateOperationError(err)
	}
	return gcp.waitForRegionOperation(ctx, svc, region, op)
}

func (gcp Gcp) DeleteNetwork(ctx context.Context, name string) error {
	svc, err := gcp.networkService(ctx)
	if err != nil {
		return err
	}
	op, err := svc.Networks.Delete(gcp.ProjectId, name).Context(ctx).Do()
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return annotateOperationError(err)
	}
	return gcp.waitForGlobalOperation(ctx, svc, op)
}

func isNotFound(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == 404
}
