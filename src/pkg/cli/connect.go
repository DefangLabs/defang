package cli

import (
	"context"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/do"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/gcp"
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
)

const DefaultCluster = login.DefaultCluster

var DefangFabric = login.DefangFabric

func Connect(ctx context.Context, cluster string) (*client.GrpcClient, error) {
	tenantName, host := login.SplitTenantHost(cluster)
	accessToken := login.GetExistingToken(cluster)
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

func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	lastColon := strings.LastIndexByte(errStr, ':')
	switch errStr[lastColon+1:] { // +1 to skip the colon and handle the case where there is no colon
	case " connection refused",
		" i/o timeout",
		" network is unreachable",
		" no such host",
		" unexpected EOF",
		" device or resource busy":
		return true
	}
	return false
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
