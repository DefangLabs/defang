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
	"github.com/bufbuild/connect-go"
)

const DefaultCluster = "fabric-prod1.defang.dev"

var DefangFabric = pkg.Getenv("DEFANG_FABRIC", DefaultCluster)

func SplitTenantHost(cluster string) (types.TenantName, string) {
	tenant := types.DEFAULT_TENANT
	parts := strings.SplitN(cluster, "@", 2)
	if len(parts) == 2 {
		tenant, cluster = types.TenantName(parts[0]), parts[1]
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
	var tenantName types.TenantName
	tenant, host := SplitTenantHost(cluster)
	if tenant != types.DEFAULT_TENANT {
		tenantName = tenant
	}
	accessToken := GetExistingToken(cluster)
	term.Debug("Using tenant", tenantName, "for cluster", host)
	grpcClient := client.NewGrpcClient(host, accessToken, tenantName)
	track.Tracker = grpcClient // Update track client

	resp, err := grpcClient.WhoAmI(ctx)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeUnavailable {
			term.Fatalf("Unable to connect; please check your internet, VPN, or firewall settings and try again.")
			term.Debug("Connection error details:", err)
		} else {
			term.Debug("Unable to validate tenant ID with server:", err)
		}
	} else if string(tenantName) != resp.Tenant {
		if tenantName != types.DEFAULT_TENANT {
			term.Debugf("Overriding tenant %q with server provided value %q", tenantName, resp.Tenant)
		}
		grpcClient.TenantName = types.TenantName(resp.Tenant)
	}
	return grpcClient
}

func NewProvider(ctx context.Context, providerID client.ProviderID, fabricClient client.FabricClient) (client.Provider, error) {
	var provider client.Provider
	term.Debugf("Creating %s provider", providerID)
	switch providerID {
	case client.ProviderAWS:
		provider = aws.NewByocProvider(ctx, fabricClient.GetTenantName())
	case client.ProviderDO:
		provider = do.NewByocProvider(ctx, fabricClient.GetTenantName())
	case client.ProviderGCP:
		provider = gcp.NewByocProvider(ctx, fabricClient.GetTenantName())
	default:
		provider = &client.PlaygroundProvider{FabricClient: fabricClient}
	}
	_, err := provider.AccountInfo(ctx)
	return provider, err
}
