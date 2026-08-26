package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

var _ client.OrphanCleaner = (*ByocAzure)(nil)

// DiscoverOrphans reports the project's resource group if it still exists after `defang down`.
// Unlike the AWS resources in byoc/aws/cleanup.go, this group is never a Pulumi teardown
// blocker: the Pulumi program deliberately retains it (see createProjectResourceGroup in the
// defang-azure provider) so the project's Key Vault and secrets survive between deploys. It is
// simply left behind until the caller explicitly asks for it to be removed.
func (b *ByocAzure) DiscoverOrphans(ctx context.Context, projectName string) ([]client.OrphanResource, error) {
	if err := b.setUpLocation(); err != nil {
		return nil, err
	}
	rgName := b.projectResourceGroupName(projectName)
	exists, err := b.driver.ResourceGroupExists(ctx, rgName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	return []client.OrphanResource{{
		ID:       "rg:" + rgName,
		Category: "rg",
		Name:     rgName,
		Action:   "delete the resource group, which also deletes the project's Key Vault and secrets",
	}}, nil
}

// CleanupOrphan deletes the project's resource group. Deleting the group cascades to delete the
// Key Vault (and anything else still inside it) along with it.
func (b *ByocAzure) CleanupOrphan(ctx context.Context, r client.OrphanResource) error {
	rgName, ok := strings.CutPrefix(r.ID, "rg:")
	if !ok || r.Category != "rg" {
		return fmt.Errorf("unsupported orphan resource %q", r.ID)
	}
	return b.driver.DeleteResourceGroup(ctx, rgName)
}
