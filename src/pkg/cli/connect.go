package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"net"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/do"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/term"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/types"
)

const DefaultCluster = "fabric-prod1.defang.dev"

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
		cluster = DefaultCluster
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

func Connect(cluster string, loader client.ProjectLoader) (*client.GrpcClient, types.TenantID) {
	accessToken, tenantId := getExistingTokenAndTenant(cluster)

	tenant, host := SplitTenantHost(cluster)
	if tenant != types.DEFAULT_TENANT {
		tenantId = tenant
	}
	term.Debug(" - Using tenant", tenantId, "for cluster", host)

	defangClient := client.NewGrpcClient(host, accessToken, tenantId, loader)
	resp, err := defangClient.WhoAmI(context.TODO()) // TODO: Should we pass in the command context?
	if err != nil {
		term.Debug(" - Unable to validate tenant ID with server:", err)
	}
	if resp != nil && tenantId != types.TenantID(resp.Tenant) {
		term.Warnf(" ! Overriding locally cached TenantID %v with server provided value %v", tenantId, resp.Tenant)
		tenantId = types.TenantID(resp.Tenant)
	}
	return defangClient, tenantId
}

func NewClient(cluster string, provider client.Provider, loader client.ProjectLoader) client.Client {
	defangClient, tenantId := Connect(cluster, loader)

	baseClient := byoc.NewByocBaseClient(defangClient, tenantId)

	switch provider {
	case client.ProviderAWS:
		term.Info(" # Using AWS provider") // HACK: # prevents errors when evaluating the shell completion script
		byocClient := aws.NewByoc(baseClient)
		return byocClient
	case client.ProviderDO:
		term.Info("# Using DO provider")
		byocClient := do.NewByoc(baseClient)
		return byocClient
	default:
		return &client.PlaygroundClient{*defangClient}
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
