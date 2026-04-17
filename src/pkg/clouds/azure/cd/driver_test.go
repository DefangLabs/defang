//go:build integration

package cd

import (
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
)

var testResourceGroupName = "crun-test-" + pkg.GetCurrentUser()
