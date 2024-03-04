package client

import (
	"os"
	"path"

	"github.com/defang-io/defang/src/pkg"
)

var (
	StateDir = path.Join(pkg.Getenv("XDG_STATE_HOME", path.Join(os.Getenv("HOME"), ".local/state")), "defang")
)

type State struct {
	AnonID string
}
