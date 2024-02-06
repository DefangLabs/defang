package cli

import (
	"errors"

	"github.com/defang-io/defang/src/pkg/types"
)

func Whoami(server string) (types.TenantID, error) {
	accessToken := GetExistingToken(server)
	if accessToken == "" {
		return "", errors.New("no valid access token found; please login first")
	}
	return TenantFromAccessToken(accessToken)
}
