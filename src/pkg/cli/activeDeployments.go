package cli

import (
	"context"
	"sort"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type PrintableActiveDeployments struct {
	Provider    string
	ProjectName string
}

func ActiveDeployments(ctx context.Context, client client.GrpcClient) error {
	response, err := client.GetActiveDeployments(ctx, &defangv1.ActiveDeploymentsRequest{})
	if err != nil {
		return err
	}

	if len(response.Deployments) == 0 {
		_, err := term.Warn("No active deployments")
		return err
	}

	// map to PrintableActiveDeployments struct
	var deployments []PrintableActiveDeployments
	for providerName, projectNames := range response.Deployments {
		for _, projectName := range projectNames.Values {
			deployments = append(deployments, PrintableActiveDeployments{
				Provider:    providerName,
				ProjectName: projectName,
			})
		}
	}

	// sort by provider then project name
	sort.Slice(deployments, func(i, j int) bool {
		if deployments[i].Provider == deployments[j].Provider {
			return deployments[i].ProjectName < deployments[j].ProjectName
		}
		return deployments[i].Provider < deployments[j].Provider
	})

	return term.Table(deployments, []string{"Provider", "ProjectName"})
}
