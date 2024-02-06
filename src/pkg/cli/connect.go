package cli

import (
	"net"
	"os"
	"strings"

	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/types"
)

const DefaultCluster = "fabric-prod1.defang.dev"

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

func Connect(cluster string, provider client.Provider) (client.Client, types.TenantID) {
	tenantId, host := SplitTenantHost(cluster)

	accessToken := GetExistingToken(cluster)
	if accessToken != "" {
		tenantId, _ = client.TenantFromAccessToken(accessToken)
	}
	Debug(" - Using tenant", tenantId, "for cluster", cluster, "and provider", provider)

	Info(" * Connecting to", host)
	defangClient := client.NewGrpcClient(host, accessToken)

	awsInEnv := os.Getenv("AWS_PROFILE") != "" || os.Getenv("AWS_REGION") != "" || os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_SECRET_ACCESS_KEY") != ""
	if provider == client.ProviderAWS || (provider == client.ProviderAuto && awsInEnv) {
		Info(" * Using AWS provider")
		if !awsInEnv {
			Warn(" ! AWS provider was selected, but AWS environment variables are not set")
		}
		byocClient := client.NewByocAWS(string(tenantId), "", defangClient) // TODO: custom domain
		return byocClient, tenantId
	}

	if awsInEnv {
		Warn(" ! Using Defang provider, but AWS environment variables were detected; use --provider")
	}
	return defangClient, tenantId
}
