package cli

import (
	"net"
	"net/http"
	"strings"

	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg/auth"
	"github.com/defang-io/defang/src/pkg/types"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
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

func Connect(cluster string) (defangv1connect.FabricControllerClient, types.TenantID) {
	tenantId, host := SplitTenantHost(cluster) // TODO: use this returned tenantId when we have no access token

	accessToken := GetExistingToken(cluster)
	if accessToken != "" {
		tenantId, _ = TenantFromAccessToken(accessToken)
	}
	Debug(" - Using tenant", tenantId, "for cluster", cluster)

	baseUrl := "http://"
	if strings.HasSuffix(host, ":443") {
		baseUrl = "https://"
	}
	baseUrl += host
	Debug(" - Connecting to", baseUrl)
	client := defangv1connect.NewFabricControllerClient(http.DefaultClient, baseUrl, connect.WithGRPC(), connect.WithInterceptors(auth.NewAuthInterceptor(accessToken)))
	Info(" * Connected to", host)
	return client, tenantId
}
