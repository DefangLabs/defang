package cli

import (
	"net/http"
	"strings"

	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/auth"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
)

func Connect(server string) (defangv1connect.FabricControllerClient, pkg.TenantID) {
	accessToken := GetExistingToken(server)
	tenantId, _ := TenantFromAccessToken(accessToken)
	_, host := SplitTenantHost(server) // TODO: use this returned tenantId when we have no access token
	Debug(" - Using tenant", tenantId, "for server", server)

	baseUrl := "http://"
	if strings.HasSuffix(server, ":443") {
		baseUrl = "https://"
	}
	baseUrl += host
	Debug(" - Connecting to", baseUrl)
	client := defangv1connect.NewFabricControllerClient(http.DefaultClient, baseUrl, connect.WithGRPC(), connect.WithInterceptors(auth.NewAuthInterceptor(accessToken)))
	Info(" * Connected to", host)
	return client, tenantId
}
