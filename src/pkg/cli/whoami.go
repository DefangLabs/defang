package cli

import fab "github.com/defang-io/defang/src/pkg"

func Whoami(server string) (fab.TenantID, error) {
	accessToken := GetExistingToken(server)
	return TenantFromAccessToken(accessToken)
}
