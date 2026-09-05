package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/azure"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/do"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/gcp"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
)

// Connect builds a client carrying the requested tenant (name or ID).
func Connect(fabricAddr string, requestedTenant types.TenantNameOrID) *client.GrpcClient {
	host := client.NormalizeHost(fabricAddr)
	term.Debugf("Using tenant %q for cluster %q", requestedTenant, host)

	accessToken := client.GetExistingToken(host)
	return client.NewGrpcClient(host, accessToken, requestedTenant)
}

func ConnectWithTenant(ctx context.Context, fabricAddr string, requestedTenant types.TenantNameOrID) (*client.GrpcClient, error) {
	grpcClient := Connect(fabricAddr, requestedTenant)

	resp, err := grpcClient.WhoAmI(ctx)
	if err != nil {
		term.Debug("Unable to validate tenant with server:", err)
		return grpcClient, err
	}

	// A DEFANG_ACCESS_TOKEN is minted for one fixed workspace; the server always resolves it
	// to that workspace regardless of the tenant header, so a mismatched --workspace/
	// DEFANG_WORKSPACE would otherwise be silently ignored. Fail instead of proceeding on the
	// wrong workspace.
	if requestedTenant.IsSet() && client.UsingAccessTokenEnv() &&
		string(requestedTenant) != resp.Tenant && string(requestedTenant) != resp.TenantId {
		return nil, fmt.Errorf("requested workspace %q does not match workspace %q associated with the DEFANG_ACCESS_TOKEN in use", requestedTenant, resp.Tenant)
	}

	grpcClient.Tenant = types.TenantLabel(resp.Tenant)
	return grpcClient, nil
}

func NewProvider(ctx context.Context, providerID client.ProviderID, fabricClient client.FabricClient, stack string) client.Provider {
	var provider client.Provider
	term.Debugf("Creating %s provider", providerID)
	switch providerID {
	case client.ProviderAWS:
		provider = aws.NewByocProvider(ctx, fabricClient.GetTenantName(), stack, fabricClient)
	case client.ProviderDO:
		provider = do.NewByocProvider(ctx, fabricClient.GetTenantName(), stack)
	case client.ProviderGCP:
		provider = gcp.NewByocProvider(ctx, fabricClient.GetTenantName(), stack)
	case client.ProviderAzure:
		provider = azure.NewByocProvider(ctx, fabricClient.GetTenantName(), stack)
	default:
		provider = client.NewPlaygroundProvider(fabricClient, stack)
	}
	return provider
}
