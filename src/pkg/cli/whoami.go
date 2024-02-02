package cli

import "github.com/defang-io/defang/src/pkg/types"

func Whoami(server string) (types.TenantID, error) {
	accessToken := GetExistingToken(server)
	return TenantFromAccessToken(accessToken)
}
