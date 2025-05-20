//go:build windows

package client

import (
	"os"
)

func userStateDir() (string, error) {
	return os.UserCacheDir() // %LocalAppData%
}
