package cli

import (
	"net/http"
	"os"
	"strings"

	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/auth"
	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
)

func Connect(server string, provider client.Provider) (client.Client, pkg.TenantID) {
	tenantId, host := SplitTenantHost(server)

	accessToken := GetExistingToken(server)
	if accessToken != "" {
		tenantId, _ = TenantFromAccessToken(accessToken)
	}
	Debug(" - Using tenant", tenantId, "for server", server, "and provider", provider)

	baseUrl := "http://"
	if strings.HasSuffix(server, ":443") {
		baseUrl = "https://"
	}
	baseUrl += host
	Debug(" - Connecting to", baseUrl)
	fabricClient := defangv1connect.NewFabricControllerClient(http.DefaultClient, baseUrl, connect.WithGRPC(), connect.WithInterceptors(auth.NewAuthInterceptor(accessToken)))
	Info(" * Connected to", host)
	defangClient := client.NewGrpcClient(fabricClient, server, accessToken)

	awsInEnv := os.Getenv("AWS_PROFILE") != "" || os.Getenv("AWS_REGION") != "" || os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_SECRET_ACCESS_KEY") != ""
	if provider == client.ProviderAWS || (provider == client.ProviderAuto && awsInEnv) {
		if !awsInEnv {
			Warn(" ! AWS provider was selected, but AWS environment variables are not set")
		}
		byocClient := client.NewByocAWS(string(tenantId), "", defangClient) // TODO: custom domain
		return byocClient, pkg.TenantID(byocClient.StackID)
	}

	if awsInEnv {
		Warn(" ! Using Defang provider, but AWS environment variables are detected; use '-P aws'")
	}
	return defangClient, tenantId
}
