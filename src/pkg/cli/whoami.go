package cli

import "github.com/defang-io/defang/src/pkg"

func Whoami(server string) (pkg.TenantID, error) {
	accessToken := GetExistingToken(server)
	return TenantFromAccessToken(accessToken)
}
