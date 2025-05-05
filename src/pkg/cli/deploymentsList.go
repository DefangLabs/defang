package cli

import (
	"context"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type PrintDeployment struct {
	Deployment string
	Provider   string
	DeployedAt string
	Region     string
}

func DeploymentsList(ctx context.Context, projectName string, client client.GrpcClient, limit uint32) error {
	response, err := client.ListDeployments(ctx, &defangv1.ListDeploymentsRequest{
		Project: projectName,
		Limit:   limit,
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
			Deployment: d.Id,
			Provider:   d.ProviderString, // TODO: use Provider
			DeployedAt: d.Timestamp.AsTime().Format(time.RFC3339),
			Region:     d.Region,
		}
	}

	return term.Table(deployments, []string{"Deployment", "Provider", "Region", "DeployedAt"})
}
