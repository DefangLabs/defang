package cli

import (
	"context"
	"net"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/do"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/gcp"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
)

const DefaultCluster = "fabric-prod1.defang.dev"

var DefangFabric = pkg.Getenv("DEFANG_FABRIC", DefaultCluster)

func SplitTenantHost(cluster string) (types.TenantID, string) {
	tenant := types.DEFAULT_TENANT
	parts := strings.SplitN(cluster, "@", 2)
	if len(parts) == 2 {
		tenant, cluster = types.TenantID(parts[0]), parts[1]
	}
	if cluster == "" {
		cluster = DefangFabric
	}
	if _, _, err := net.SplitHostPort(cluster); err != nil {
		cluster = cluster + ":443" // default to https
	}
	return tenant, cluster
}

func NewGrpcClient(ctx context.Context, cluster string) client.GrpcClient {
	var tenantId types.TenantID
	tenant, host := SplitTenantHost(cluster)
	if tenant != types.DEFAULT_TENANT {
		tenantId = tenant
	}
	accessToken := GetExistingToken(cluster)
	term.Debug("Using tenant", tenantId, "for cluster", host)
	grpcClient := client.NewGrpcClient(host, accessToken, tenantId)

	resp, err := grpcClient.WhoAmI(ctx)
	if err != nil {
		term.Debug("Unable to validate tenant ID with server:", err)
	} else if string(tenantId) != resp.Tenant {
		if tenantId != types.DEFAULT_TENANT {
			term.Debugf("Overriding TenantID %q with server provided value %q", tenantId, resp.Tenant)
		}
		grpcClient.TenantID = types.TenantID(resp.Tenant)
	}
	track.Fabric = grpcClient // Update track client
	return grpcClient
}

func NewProvider(ctx context.Context, providerID client.ProviderID, grpcClient client.GrpcClient) (client.Provider, error) {
	var provider client.Provider
	term.Debugf("Creating provider %q", providerID)
	switch providerID {
	case client.ProviderAWS:
		provider = aws.NewByocProvider(ctx, grpcClient.TenantID)
	case client.ProviderDO:
		provider = do.NewByocProvider(ctx, grpcClient.TenantID)
	case client.ProviderGCP:
		provider = gcp.NewByocProvider(ctx, grpcClient.TenantID)
	default:
		provider = &client.PlaygroundProvider{GrpcClient: grpcClient}
	}
	_, err := provider.AccountInfo(ctx)
	return provider, err
}
