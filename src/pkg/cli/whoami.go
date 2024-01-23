package cli

import (
	"github.com/defang-io/defang/src/pkg"
)

func Whoami(server string) (pkg.TenantID, error) {
	// TODO: use WhoAmI rpc
	accessToken := GetExistingToken(server)
	return TenantFromAccessToken(accessToken)
}
