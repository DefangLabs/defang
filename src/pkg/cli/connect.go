package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/do"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/gcp"
	"github.com/DefangLabs/defang/src/pkg/cluster"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
)

func Connect(ctx context.Context, addr string) (*client.GrpcClient, error) {
	tenantName, host := cluster.SplitTenantHost(addr)
	accessToken := cluster.GetExistingToken(addr)
	term.Debug("Using tenant", tenantName, "for cluster", host)
	grpcClient := client.NewGrpcClient(host, accessToken, tenantName)
	track.Tracker = grpcClient // Update track client

	resp, err := grpcClient.WhoAmI(ctx)
	if err != nil {
		term.Debug("Unable to validate tenant ID with server:", err)
	} else if string(tenantName) != resp.Tenant {
		if tenantName != types.DEFAULT_TENANT {
			term.Debugf("Overriding tenant %q with server provided value %q", tenantName, resp.Tenant)
		}
		grpcClient.TenantName = types.TenantName(resp.Tenant)
	}
	return grpcClient, err
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
