package cli

import (
	"net/http"
	"strings"

	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/auth"
	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
)

func Connect(server, provider string) (client.Client, pkg.TenantID) {
	tenantId, host := SplitTenantHost(server)

	accessToken := GetExistingToken(server)
	if accessToken != "" {
		tenantId, _ = TenantFromAccessToken(accessToken)
	}
	Debug(" - Using tenant", tenantId, "for server", server)

	baseUrl := "http://"
	if strings.HasSuffix(server, ":443") {
		baseUrl = "https://"
	}
	baseUrl += host
	Debug(" - Connecting to", baseUrl)
	fabricClient := defangv1connect.NewFabricControllerClient(http.DefaultClient, baseUrl, connect.WithGRPC(), connect.WithInterceptors(auth.NewAuthInterceptor(accessToken)))
	Info(" * Connected to", host)
	defClient := client.NewGrpcClient(fabricClient, server)

	if provider == "aws" {
		Debug(" - Using AWS provider")
		byocClient := client.NewByocAWS(string(tenantId), "", defClient, GetExistingToken) // TODO: custom domain
		return byocClient, pkg.TenantID(byocClient.StackID)
	}

	return defClient, tenantId
}
