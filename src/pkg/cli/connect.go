package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/do"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
)

const DefaultCluster = "fabric-prod1.defang.dev"

var DefangFabric = pkg.Getenv("DEFANG_FABRIC", DefaultCluster)

// Deprecated: should use grpc to get the tenant ID
func GetTenantID(cluster string) types.TenantID {
	if tenantId, _ := SplitTenantHost(cluster); tenantId != types.DEFAULT_TENANT {
		return tenantId
	}

	_, tenantId := getExistingTokenAndTenant(cluster)
	return tenantId
}

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

func getExistingTokenAndTenant(cluster string) (string, types.TenantID) {
	var tenantId types.TenantID
	accessToken := GetExistingToken(cluster)
	if accessToken != "" {
		// HACK: don't rely on info in token
		tenantId, _, _ = tenantFromAccessToken(accessToken)
	}
	return accessToken, tenantId
}

func Connect(cluster string, loader client.ProjectLoader) client.GrpcClient {
	accessToken, tenantId := getExistingTokenAndTenant(cluster)

	tenant, host := SplitTenantHost(cluster)
	if tenant != types.DEFAULT_TENANT {
		tenantId = tenant
	}
	term.Debug("Using tenant", tenantId, "for cluster", host)

	return client.NewGrpcClient(host, accessToken, tenantId, loader)
}

func NewClient(ctx context.Context, cluster string, providerID client.ProviderID, loader client.ProjectLoader) (client.FabricClient, client.Provider) {
	grpcClient := Connect(cluster, loader)
	track.Fabric = grpcClient // Update track client

	// Determine the current tenant ID
	resp, err := grpcClient.WhoAmI(ctx)
	if err != nil {
		term.Debug("Unable to validate tenant ID with server:", err)
	}
	tenantId := grpcClient.TenantID
	if resp != nil && string(tenantId) != resp.Tenant {
		term.Warnf("Overriding locally cached TenantID %q with server provided value %q", tenantId, resp.Tenant)
		tenantId = types.TenantID(resp.Tenant)
	}

	switch providerID {
	case client.ProviderAWS:
		term.Info("Using AWS provider")
		byocClient, err := aws.NewByocClient(ctx, grpcClient, tenantId)
		if err != nil {
			term.Fatal(err)
		}
		return byocClient
	case client.ProviderDO:
		term.Info("Using DigitalOcean provider")
		byocProvider, err := do.NewByocProvider(ctx, grpcClient, tenantId)
		if err != nil {
			term.Fatal(err)
		}
		return grpcClient, byocProvider
	default:
		term.Info("Using Defang Playground; consider using BYOC (https://s.defang.io/byoc)")
		return grpcClient, &client.PlaygroundProvider{GrpcClient: grpcClient}
	}
}

// Deprecated: don't rely on info in token
func tenantFromAccessToken(at string) (types.TenantID, string, error) {
	parts := strings.Split(at, ".")
	if len(parts) != 3 {
		return "", "", errors.New("not a JWT")
	}
	var claims struct {
		Iss string `json:"iss"`
		Sub string `json:"sub"`
	}
	bytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", err
	}
	err = json.Unmarshal(bytes, &claims)
	return types.TenantID(claims.Sub), claims.Iss, err
}
