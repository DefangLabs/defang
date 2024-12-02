package cli

import (
	"context"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type PrintDeployment struct {
	Id         string
	Provider   string
	DeployedAt string
}

func DeploymentsList(ctx context.Context, loader client.Loader, client client.GrpcClient) error {
	projectName, err := loader.LoadProjectName(ctx)
	if err != nil {
		return err
	}

	response, err := client.ListDeployments(ctx, &defangv1.ListDeploymentsRequest{
		Project: projectName,
	})
	if err != nil {
		return err
	}

	numDeployments := len(response.Deployments)
	if numDeployments == 0 {
		_, err := term.Warnf("No deployments found for project %q", projectName)
		return err
	}

	// map to Deployment struct
	deployments := make([]PrintDeployment, numDeployments)
	for i, d := range response.Deployments {
		deployments[i] = PrintDeployment{
			Id:         d.Id,
			Provider:   d.Provider,
			DeployedAt: d.Timestamp.AsTime().Format(time.RFC3339),
		}
	}

	return term.Table(deployments, []string{"Id", "Provider", "DeployedAt"})
}
