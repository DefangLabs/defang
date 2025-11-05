package actions

import (
	"context"
	"os"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

func SetGCPByocProvider(ctx context.Context, providerId *client.ProviderID, cluster string, projectID string) error {
	err := os.Setenv("GCP_PROJECT_ID", projectID)
	if err != nil {
		return err
	}

	fabric, err := common.Connect(ctx, cluster)
	if err != nil {
		return err
	}

	_, err = common.CheckProviderConfigured(ctx, fabric, client.ProviderGCP, "", 0)
	if err != nil {
		return err
	}

	*providerId = client.ProviderGCP

	//FIXME: Should not be setting both the global var and env var
	err = os.Setenv("DEFANG_PROVIDER", "gcp")
	if err != nil {
		return err
	}

	return nil
}
