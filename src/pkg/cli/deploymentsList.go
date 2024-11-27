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

	// map to Deployment struct
	deployments := make([]PrintDeployment, 0, len(response.Deployments))
	for _, d := range response.Deployments {
		deployments = append(deployments, PrintDeployment{
			Id:         d.Id,
			Provider:   d.Provider,
			DeployedAt: d.Timestamp.AsTime().Format(time.RFC3339),
		})
	}

	return term.Table(deployments, []string{"Id", "Provider", "DeployedAt"})
}
