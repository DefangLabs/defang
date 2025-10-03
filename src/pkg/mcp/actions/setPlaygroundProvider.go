package actions

import (
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

// thin wrapper to set playground provider to match other provider set ups
func SetPlaygroundProvider(providerId *client.ProviderID) error {
	*providerId = client.ProviderDefang
	return os.Setenv("DEFANG_PROVIDER", "defang")
}
