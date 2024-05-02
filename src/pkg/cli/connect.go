package cli

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"strings"

	"github.com/defang-io/defang/src/pkg/cli/client/byoc/clouds"
	"github.com/defang-io/defang/src/pkg/term"

	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/types"
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

func Connect(cluster string) (*client.GrpcClient, types.TenantID) {
	accessToken, tenantId := getExistingTokenAndTenant(cluster)

	tenant, host := SplitTenantHost(cluster)
	if tenant != types.DEFAULT_TENANT {
		tenantId = tenant
	}
	term.Debug(" - Using tenant", tenantId, "for cluster", host)

	return client.NewGrpcClient(host, accessToken), tenantId
}

func NewClient(cluster string, provider client.Provider) client.Client {
	defangClient, tenantId := Connect(cluster)

	if provider == client.ProviderAWS {
		term.Info(" # Using AWS provider") // HACK: # prevents errors when evaluating the shell completion script
		byocClient := clouds.NewByocAWS(tenantId, defangClient)
		return byocClient
	}

	return defangClient
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
