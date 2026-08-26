package client

import "context"

// OrphanResource is a cloud resource left behind after `defang down` that blocks Pulumi from
// finishing cleanup on a subsequent run. The remediation (Action) is the minimum needed to
// unblock Pulumi, not a direct delete of the resource itself.
type OrphanResource struct {
	ID       string // opaque handle, stable within a single DiscoverOrphans call
	Category string // e.g. "alb", "rds", "dns", "ecr"
	Name     string // human-readable resource name
	Action   string // what CleanupOrphan will do, e.g. "disable deletion protection"
}

// OrphanCleaner is implemented by providers that can find and unblock resources retained after
// `defang down`. It is an optional capability (currently AWS only), so callers type-assert the
// provider to this interface rather than it being part of the core Provider interface.
//
// CleanupOrphan must be called with a resource returned by the most recent DiscoverOrphans call
// on the same provider instance.
type OrphanCleaner interface {
	DiscoverOrphans(ctx context.Context, projectName string) ([]OrphanResource, error)
	CleanupOrphan(ctx context.Context, r OrphanResource) error
}
