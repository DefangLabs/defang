package gcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	compute "google.golang.org/api/compute/v1"
)

// mockNetworkDriver implements only the network-cleanup subset of gcpDriver. The embedded
// gcpDriver interface satisfies the rest of the surface, so any unexpected call panics loudly
// instead of silently returning a zero value.
type mockNetworkDriver struct {
	gcpDriver
	networks  []*compute.Network
	subnets   map[string][]*compute.Subnetwork // keyed by network self-link
	templates map[string][]*compute.InstanceTemplate
	ranges    map[string][]*compute.Address
	usage     map[string]gcp.NetworkUsage
	cloudRun  map[string][]string
	usageErr  error

	deleted []string // every mutation, in call order
}

func (m *mockNetworkDriver) ListNetworksByPrefix(ctx context.Context, prefix string) ([]*compute.Network, error) {
	var out []*compute.Network
	for _, n := range m.networks {
		if strings.HasPrefix(n.Name, prefix) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (m *mockNetworkDriver) ListSubnetworks(ctx context.Context, network *compute.Network) ([]*compute.Subnetwork, error) {
	return m.subnets[network.SelfLink], nil
}

func (m *mockNetworkDriver) ListInstanceTemplatesForNetwork(ctx context.Context, link string) ([]*compute.InstanceTemplate, error) {
	return m.templates[link], nil
}

func (m *mockNetworkDriver) ListPeeringRangesForNetwork(ctx context.Context, link string) ([]*compute.Address, error) {
	return m.ranges[link], nil
}

func (m *mockNetworkDriver) GetNetworkUsage(ctx context.Context, link string) (gcp.NetworkUsage, error) {
	if m.usageErr != nil {
		return gcp.NetworkUsage{}, m.usageErr
	}
	return m.usage[link], nil
}

func (m *mockNetworkDriver) GetCloudRunNetworkUsage(ctx context.Context, link string, subnets []string) ([]string, error) {
	return m.cloudRun[link], nil
}

func (m *mockNetworkDriver) RemoveNetworkPeering(ctx context.Context, networkName, peeringName string) error {
	m.deleted = append(m.deleted, "peering:"+networkName+"/"+peeringName)
	return nil
}

func (m *mockNetworkDriver) DeleteInstanceTemplate(ctx context.Context, name string) error {
	m.deleted = append(m.deleted, "template:"+name)
	return nil
}

func (m *mockNetworkDriver) DeleteGlobalAddress(ctx context.Context, name string) error {
	m.deleted = append(m.deleted, "address:"+name)
	return nil
}

func (m *mockNetworkDriver) DeleteSubnetwork(ctx context.Context, region, name string) error {
	m.deleted = append(m.deleted, "subnet:"+region+"/"+name)
	return nil
}

func (m *mockNetworkDriver) DeleteNetwork(ctx context.Context, name string) error {
	m.deleted = append(m.deleted, "network:"+name)
	return nil
}

const vpcLink = "https://www.googleapis.com/compute/v1/projects/p/global/networks/myproj-vpc-abc1234"

// fullDriver is a driver for a project with Postgres (peering plus reserved range) and a Compute
// Engine service (instance template), which is the case that exercises every category.
func fullDriver() *mockNetworkDriver {
	network := &compute.Network{
		Name:     "myproj-vpc-abc1234",
		SelfLink: vpcLink,
		Peerings: []*compute.NetworkPeering{{Name: "servicenetworking-googleapis-com"}},
	}
	return &mockNetworkDriver{
		networks: []*compute.Network{network},
		subnets: map[string][]*compute.Subnetwork{vpcLink: {{
			Name:     "myproj-subnet-def5678",
			SelfLink: "https://www.googleapis.com/compute/v1/projects/p/regions/us-central1/subnetworks/myproj-subnet-def5678",
			Region:   "https://www.googleapis.com/compute/v1/projects/p/regions/us-central1",
		}}},
		templates: map[string][]*compute.InstanceTemplate{vpcLink: {{Name: "svc-instance-template-9999"}}},
		ranges:    map[string][]*compute.Address{vpcLink: {{Name: "myproj-peering-ip-ffff"}}},
	}
}

func newTestByoc(driver gcpDriver) *ByocGcp {
	b := &ByocGcp{driver: driver}
	b.ByocBaseClient = &byoc.ByocBaseClient{PulumiStack: "prod"}
	return b
}

// The order is the whole point: GCP refuses to delete the subnet while a template or peering
// holds it, and the network while the subnet exists.
func TestDiscoverOrphansOrder(t *testing.T) {
	b := newTestByoc(fullDriver())
	resources, err := b.DiscoverOrphans(context.Background(), "myproj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{
		categoryInstanceTemplate,
		categoryPeering,
		categoryPeeringRange,
		categorySubnet,
		categoryNetwork,
	}
	if len(resources) != len(want) {
		t.Fatalf("got %d resources, want %d: %+v", len(resources), len(want), resources)
	}
	for i, category := range want {
		if resources[i].Category != category {
			t.Errorf("resource %d: got category %q, want %q", i, resources[i].Category, category)
		}
	}
}

// Cleaning up in the order given must reach the real driver in that same order.
func TestCleanupOrphanAppliesInOrder(t *testing.T) {
	driver := fullDriver()
	b := newTestByoc(driver)
	resources, err := b.DiscoverOrphans(context.Background(), "myproj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range resources {
		if err := b.CleanupOrphan(context.Background(), r); err != nil {
			t.Fatalf("cleanup of %s failed: %v", r.ID, err)
		}
	}
	want := []string{
		"template:svc-instance-template-9999",
		"peering:myproj-vpc-abc1234/servicenetworking-googleapis-com",
		"address:myproj-peering-ip-ffff",
		"subnet:us-central1/myproj-subnet-def5678",
		"network:myproj-vpc-abc1234",
	}
	if strings.Join(driver.deleted, ",") != strings.Join(want, ",") {
		t.Errorf("wrong calls or order:\n got %v\nwant %v", driver.deleted, want)
	}
}

// The network name carries no stack, so a prefix match can hit a live stack's VPC. Anything still
// attached must veto the whole network.
func TestDiscoverOrphansSkipsNetworkInUse(t *testing.T) {
	for name, driver := range map[string]*mockNetworkDriver{
		"instance":        {usage: map[string]gcp.NetworkUsage{vpcLink: {Instances: []string{"vm-1"}}}},
		"forwarding rule": {usage: map[string]gcp.NetworkUsage{vpcLink: {ForwardingRules: []string{"fr-1"}}}},
		"cloud run":       {cloudRun: map[string][]string{vpcLink: {"svc-1"}}},
	} {
		t.Run(name, func(t *testing.T) {
			full := fullDriver()
			full.usage = driver.usage
			full.cloudRun = driver.cloudRun
			b := newTestByoc(full)
			resources, err := b.DiscoverOrphans(context.Background(), "myproj")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(resources) != 0 {
				t.Errorf("expected an in-use network to be skipped entirely, got %+v", resources)
			}
		})
	}
}

// If usage cannot be established the network must be left alone, not assumed idle.
func TestDiscoverOrphansSkipsOnUsageError(t *testing.T) {
	driver := fullDriver()
	driver.usageErr = errors.New("permission denied")
	b := newTestByoc(driver)
	resources, err := b.DiscoverOrphans(context.Background(), "myproj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("expected no resources when usage is unknown, got %+v", resources)
	}
}

// A stack with no Postgres and no Compute Engine service leaves only the network and its subnet.
func TestDiscoverOrphansMinimalStack(t *testing.T) {
	driver := fullDriver()
	driver.templates = nil
	driver.ranges = nil
	driver.networks[0].Peerings = nil
	b := newTestByoc(driver)
	resources, err := b.DiscoverOrphans(context.Background(), "myproj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 2 || resources[0].Category != categorySubnet || resources[1].Category != categoryNetwork {
		t.Errorf("expected subnet then network, got %+v", resources)
	}
}

// Another project's VPC must not be picked up by the prefix.
func TestDiscoverOrphansIgnoresOtherProjects(t *testing.T) {
	driver := fullDriver()
	driver.networks = append(driver.networks, &compute.Network{Name: "otherproj-vpc-zzzz", SelfLink: "link/otherproj-vpc-zzzz"})
	b := newTestByoc(driver)
	resources, err := b.DiscoverOrphans(context.Background(), "myproj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range resources {
		if strings.HasPrefix(r.Name, "otherproj") {
			t.Errorf("picked up another project's resource: %+v", r)
		}
	}
}

// A stale resource ID must be rejected rather than acted on, since the details map is rebuilt by
// every discovery call.
func TestCleanupOrphanRejectsUnknownID(t *testing.T) {
	b := newTestByoc(fullDriver())
	if _, err := b.DiscoverOrphans(context.Background(), "myproj"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err := b.CleanupOrphan(context.Background(), client.OrphanResource{ID: "network:stale"})
	if err == nil || !strings.Contains(err.Error(), "run discovery again") {
		t.Errorf("expected a rejection naming rediscovery, got %v", err)
	}
}

func TestNetworkNamePrefixes(t *testing.T) {
	// Both naming shapes must be found. The current one comes from the CD's autonaming pattern
	// "<lower(prefix)>-${project}-${stack}-${name}-${hex(7)}"; a live cycle against
	// defang-playground-dev produced "defang-cdtest-min-local1-vpc-be420a3". The legacy CD used
	// "<project>-vpc-<hex>", and those are the networks most likely to have already leaked.
	b := &ByocGcp{}
	b.ByocBaseClient = &byoc.ByocBaseClient{Prefix: "Defang", PulumiStack: "local1"}

	matches := func(name string) bool {
		for _, prefix := range b.networkNamePrefixes("html-css-js") {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
		return false
	}

	for _, name := range []string{
		"defang-html-css-js-local1-vpc-be420a3", // current
		"html-css-js-vpc-e99e23a",               // legacy
		"html-css-js-vpc",
	} {
		if !matches(name) {
			t.Errorf("%q should have matched one of %q", name, b.networkNamePrefixes("html-css-js"))
		}
	}

	// A prefix must not reach a shorter sibling project, nor another stack's network under the
	// current naming — the stack sits between the project and "vpc" precisely so it cannot.
	for _, name := range []string{
		"html-vpc-abc",
		"defang-html-css-js-other-vpc-be420a3",
	} {
		if matches(name) {
			t.Errorf("%q should not have matched any of %q", name, b.networkNamePrefixes("html-css-js"))
		}
	}

	// An empty prefix must not produce a leading hyphen.
	b.ByocBaseClient = &byoc.ByocBaseClient{PulumiStack: "local1"}
	if got := b.networkNamePrefixes("html-css-js")[0]; got != "html-css-js-local1-vpc" {
		t.Errorf("empty prefix produced %q", got)
	}
}

// The 1-2 hour Cloud Run window is expected, not exceptional, so it must be explained rather than
// surfaced as a bare API error — and only for the resources it can actually affect.
func TestAnnotateCleanupError(t *testing.T) {
	inUse := errors.New("wrapped: " + gcp.ErrResourceInUse.Error())
	inUse = errors.Join(gcp.ErrResourceInUse, inUse)

	got := annotateCleanupError(categorySubnet, inUse)
	if !strings.Contains(got.Error(), "1-2 hours") {
		t.Errorf("subnet error should explain the release window, got %q", got)
	}
	if !errors.Is(got, gcp.ErrResourceInUse) {
		t.Error("annotation must preserve the sentinel for errors.Is")
	}

	got = annotateCleanupError(categoryPeering, inUse)
	if strings.Contains(got.Error(), "1-2 hours") {
		t.Errorf("the release window does not apply to a peering, got %q", got)
	}

	if annotateCleanupError(categorySubnet, nil) != nil {
		t.Error("nil must stay nil")
	}

	other := errors.New("quota exceeded")
	if !errors.Is(annotateCleanupError(categoryNetwork, other), other) {
		t.Error("unrelated errors must pass through untouched")
	}
}
