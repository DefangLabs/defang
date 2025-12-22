package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/do"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/gcp"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
)

// Connect builds a client carrying the requested tenant (name or ID).
func Connect(cluster string, requestedTenant types.TenantNameOrID) *client.GrpcClient {
	host := client.NormalizeHost(cluster)
	term.Debugf("Using tenant %q for cluster %q", requestedTenant, host)

	accessToken := client.GetExistingToken(host)
	return client.NewGrpcClient(host, accessToken, requestedTenant)
}

func ConnectWithTenant(ctx context.Context, addr string, requestedTenant types.TenantNameOrID) (*client.GrpcClient, error) {
	grpcClient := Connect(addr, requestedTenant)

	resp, err := grpcClient.WhoAmI(ctx)
	if err != nil {
		term.Debug("Unable to validate tenant with server:", err)
		return grpcClient, err
	}

	grpcClient.Tenant = types.TenantName(resp.Tenant)
	return grpcClient, nil
}

func NewProvider(ctx context.Context, providerID client.ProviderID, fabricClient client.FabricClient, stack string) client.Provider {
	var provider client.Provider
	term.Debugf("Creating %s provider", providerID)
	switch providerID {
	case client.ProviderAWS:
		provider = aws.NewByocProvider(ctx, fabricClient.GetTenantName(), stack)
	case client.ProviderDO:
		provider = do.NewByocProvider(ctx, fabricClient.GetTenantName(), stack)
	case client.ProviderGCP:
		provider = gcp.NewByocProvider(ctx, fabricClient.GetTenantName(), stack)
	default:
		provider = &client.PlaygroundProvider{FabricClient: fabricClient}
	}
	return provider
}
