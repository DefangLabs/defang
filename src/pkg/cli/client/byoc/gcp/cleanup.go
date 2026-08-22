package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	"github.com/DefangLabs/defang/src/pkg/term"
	compute "google.golang.org/api/compute/v1"
)

var _ client.OrphanCleaner = (*ByocGcp)(nil)

// Orphan categories, in the order they must be cleaned up. GCP refuses to delete a network while
// a subnet exists, a subnet while an instance template or a Cloud Run service holds it, and a
// reserved range while the peering that uses it is still attached.
const (
	categoryInstanceTemplate = "instance-template"
	categoryPeering          = "peering"
	categoryPeeringRange     = "peering-range"
	categorySubnet           = "subnet"
	categoryNetwork          = "network"
)

// gcpOrphanDetail holds the cloud-specific handles needed to clean up an OrphanResource. Only the
// fields relevant to the resource's category are populated.
type gcpOrphanDetail struct {
	category    string
	name        string // resource name, for the delete call
	networkName string // for categoryPeering: the network to remove the peering from
	region      string // for categorySubnet: subnetwork deletes are regional
}

// networkNamePrefixes returns the prefixes a project's VPC network name can have.
//
// GCP networks and subnetworks have no labels field, so unlike every other resource in the stack
// they cannot be found by the defang-project and defang-stack labels that cd/program/gcp.go sets.
// The name is the only handle, and three shapes of it exist in live projects — all three were
// observed side by side in defang-playground-dev:
//
//   - Current. The CD sets `pulumi:autonaming` to `<lower(prefix)>-${project}-${stack}-${name}-${hex(7)}`
//     (cd/config.go) and the program's logical name for the network is "vpc", giving
//     `defang-<project>-<stack>-vpc-<hex7>`.
//   - Same autonaming, older logical name. The program used to call the network `<project>-vpc`,
//     so `${name}` carried the project too: `defang-<project>-<stack>-<project>-vpc-<hex7>`.
//   - Legacy CD. `<project>-vpc-<hex>`, with neither prefix nor stack.
//
// The last two are the shapes most likely to have already leaked, so missing them would miss the
// backlog this command exists to clear. The legacy prefix carries no stack, so two stacks of one
// compose project share it — which is why a prefix match alone never authorises a delete: see the
// in-use check in DiscoverOrphans.
func (b *ByocGcp) networkNamePrefixes(projectName string) []string {
	var prefix string
	if b.Prefix != "" {
		prefix = strings.ToLower(b.Prefix) + "-"
	}
	stackPrefix := prefix + projectName + "-" + b.PulumiStack + "-"
	return []string{
		stackPrefix + "vpc",
		stackPrefix + projectName + "-vpc",
		projectName + "-vpc",
	}
}

// DiscoverOrphans finds the GCP networking resources that `defang down` leaves behind. The Pulumi
// program creates the network, subnet, service networking connection and instance templates with
// RetainOnDelete, so a successful `down` removes them from Pulumi state but not from GCP, and the
// leaked networks eventually exhaust the project's NETWORKS quota.
//
// The returned resources are ordered: they must be cleaned up in the order given, because GCP
// enforces the dependencies between them.
func (b *ByocGcp) DiscoverOrphans(ctx context.Context, projectName string) ([]client.OrphanResource, error) {
	b.orphans = map[string]gcpOrphanDetail{}
	var resources []client.OrphanResource
	add := func(id string, r client.OrphanResource, d gcpOrphanDetail) {
		r.ID = id
		b.orphans[id] = d
		resources = append(resources, r)
	}

	// One network can match both prefixes only if the legacy shape is also the current one, but
	// dedupe anyway: offering the same network twice would make the second pass fail on a 404.
	seen := map[string]bool{}
	var networks []*compute.Network
	for _, prefix := range b.networkNamePrefixes(projectName) {
		found, err := b.driver.ListNetworksByPrefix(ctx, prefix)
		if err != nil {
			return nil, err
		}
		for _, network := range found {
			if !seen[network.Name] {
				seen[network.Name] = true
				networks = append(networks, network)
			}
		}
	}

	for _, network := range networks {
		subnets, err := b.driver.ListSubnetworks(ctx, network)
		if err != nil {
			term.Warnf("cleanup: could not list subnetworks of %s: %v", network.Name, err)
			continue
		}

		// A network with anything live attached belongs to a running stack, not to a torn-down
		// one. Skipping it is what makes the name-prefix match above safe: the prefix carries no
		// stack, so without this check a cleanup could tear the networking out from under
		// another stack of the same project.
		inUse, err := b.networkInUse(ctx, network, subnets)
		if err != nil {
			term.Warnf("cleanup: could not check whether %s is still in use: %v", network.Name, err)
			continue // never offer a network whose usage could not be established
		}
		if inUse != "" {
			term.Debugf("cleanup: skipping network %s, still in use by %s", network.Name, inUse)
			continue
		}

		// Instance templates hold the subnet they reference, so they go first.
		templates, err := b.driver.ListInstanceTemplatesForNetwork(ctx, network.SelfLink)
		if err != nil {
			term.Warnf("cleanup: could not list instance templates for %s: %v", network.Name, err)
		}
		for _, tmpl := range templates {
			add("template:"+tmpl.Name, client.OrphanResource{
				Category: categoryInstanceTemplate,
				Name:     tmpl.Name,
				Action:   "delete the instance template that holds the subnet",
			}, gcpOrphanDetail{category: categoryInstanceTemplate, name: tmpl.Name})
		}

		// Then the peerings. Removing a peering through the Compute API is the only way to drop
		// a service networking connection: see RemoveNetworkPeering.
		for _, peering := range network.Peerings {
			add("peering:"+network.Name+"/"+peering.Name, client.OrphanResource{
				Category: categoryPeering,
				Name:     peering.Name,
				Action:   fmt.Sprintf("remove the VPC peering from network %s, releasing its reserved IP range", network.Name),
			}, gcpOrphanDetail{category: categoryPeering, name: peering.Name, networkName: network.Name})
		}

		// Then the ranges the peerings reserved, which only become deletable once the peering is
		// gone.
		ranges, err := b.driver.ListPeeringRangesForNetwork(ctx, network.SelfLink)
		if err != nil {
			term.Warnf("cleanup: could not list reserved peering ranges for %s: %v", network.Name, err)
		}
		for _, addr := range ranges {
			add("address:"+addr.Name, client.OrphanResource{
				Category: categoryPeeringRange,
				Name:     addr.Name,
				Action:   "delete the reserved IP range that was allocated for VPC peering",
			}, gcpOrphanDetail{category: categoryPeeringRange, name: addr.Name})
		}

		// Then the subnets. This is the step that can hit the Cloud Run release window.
		for _, subnet := range subnets {
			add("subnet:"+subnet.Name, client.OrphanResource{
				Category: categorySubnet,
				Name:     subnet.Name,
				Action:   "delete the subnetwork",
			}, gcpOrphanDetail{category: categorySubnet, name: subnet.Name, region: gcp.ResourceName(subnet.Region)})
		}

		// And finally the network itself.
		add("network:"+network.Name, client.OrphanResource{
			Category: categoryNetwork,
			Name:     network.Name,
			Action:   "delete the VPC network, freeing one of the project's 30 network quota slots",
		}, gcpOrphanDetail{category: categoryNetwork, name: network.Name})
	}

	return resources, nil
}

// networkInUse returns a description of what still uses the network, or "" when nothing does.
func (b *ByocGcp) networkInUse(ctx context.Context, network *compute.Network, subnets []*compute.Subnetwork) (string, error) {
	usage, err := b.driver.GetNetworkUsage(ctx, network.SelfLink)
	if err != nil {
		return "", err
	}
	subnetLinks := make([]string, 0, len(subnets))
	for _, subnet := range subnets {
		subnetLinks = append(subnetLinks, subnet.SelfLink)
	}
	services, err := b.driver.GetCloudRunNetworkUsage(ctx, network.SelfLink, subnetLinks)
	if err != nil {
		return "", err
	}
	usage.CloudRun = services
	if usage.InUse() {
		return usage.Describe(), nil
	}
	return "", nil
}

// CleanupOrphan deletes one resource returned by the most recent DiscoverOrphans call. Resources
// must be cleaned up in the order DiscoverOrphans returned them.
func (b *ByocGcp) CleanupOrphan(ctx context.Context, r client.OrphanResource) error {
	detail, ok := b.orphans[r.ID]
	if !ok {
		return fmt.Errorf("unknown resource %q; run discovery again before cleaning up", r.ID)
	}

	var err error
	switch detail.category {
	case categoryInstanceTemplate:
		err = b.driver.DeleteInstanceTemplate(ctx, detail.name)
	case categoryPeering:
		err = b.driver.RemoveNetworkPeering(ctx, detail.networkName, detail.name)
	case categoryPeeringRange:
		err = b.driver.DeleteGlobalAddress(ctx, detail.name)
	case categorySubnet:
		err = b.driver.DeleteSubnetwork(ctx, detail.region, detail.name)
	case categoryNetwork:
		err = b.driver.DeleteNetwork(ctx, detail.name)
	default:
		return fmt.Errorf("unsupported orphan category %q", detail.category)
	}
	return annotateCleanupError(detail.category, err)
}

// annotateCleanupError explains the one failure that is expected rather than exceptional. GCP
// holds a subnet's IP addresses for 1-2 hours after the last Cloud Run service using it is
// deleted, and refuses the subnet delete until it has released them. Nothing can wait that out
// inside a command, so the only useful response is to say when to come back.
func annotateCleanupError(category string, err error) error {
	if err == nil || !errors.Is(err, gcp.ErrResourceInUse) {
		return err
	}
	if category == categorySubnet || category == categoryNetwork {
		return fmt.Errorf("%w\nCloud Run releases a subnet's IP addresses 1-2 hours after the last service using it is deleted. Re-run `defang cleanup` after that to finish removing the network", err)
	}
	return err
}
