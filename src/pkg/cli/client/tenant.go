package client

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/defang-io/defang/src/pkg/types"
)

func TenantFromAccessToken(at string) (types.TenantID, error) {
	parts := strings.Split(at, ".")
	if len(parts) != 3 {
		return "", errors.New("not a JWT")
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	bytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(bytes, &claims)
	return types.TenantID(claims.Sub), err
}
