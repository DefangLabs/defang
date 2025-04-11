package cli

import (
	"context"
	"slices"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type PrintActiveDeployment struct {
	Provider    string
	ProjectName string
}

func ActiveDeployments(ctx context.Context, client client.GrpcClient) error {
	response, err := client.GetActiveDeployments(ctx, &defangv1.ActiveDeploymentsRequest{})
	if err != nil {
		return err
	}

	numDeployments := len(response.Deployments)
	if numDeployments == 0 {
		_, err := term.Warn("No active deployments")
		return err
	}

	// map to Deployment struct
	deployments := []PrintActiveDeployment{}
	i := 0
	for providerName, projectNames := range response.Deployments {
		for _, projectName := range projectNames.Values {
			deployments = append(deployments, PrintActiveDeployment{
				Provider:    providerName,
				ProjectName: projectName,
			})
			i++
		}
	}

	// sort by provider then project name
	slices.SortFunc(deployments, func(i, j PrintActiveDeployment) int {
		switch {
		case i.Provider == j.Provider:
			if i.ProjectName < j.ProjectName {
				return -1
			}
			return 1
		case i.Provider < j.Provider:
			return -1
		default:
			return 1
		}
	})

	return term.Table(deployments, []string{"Provider", "ProjectName"})
}
