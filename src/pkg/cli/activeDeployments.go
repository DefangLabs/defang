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
	Region      string
}

func ActiveDeployments(ctx context.Context, client client.GrpcClient) error {
	response, err := client.ListDeployments(ctx, &defangv1.ListDeploymentsRequest{
		Type: *defangv1.DeploymentListType_DEPLOYMENT_LIST_TYPE_ACTIVE.Enum(),
	})
	if err != nil {
		return err
	}

	numDeployments := len(response.Deployments)
	if numDeployments == 0 {
		_, err := term.Warn("No active deployments found")
		return err
	}

	// map to PrintableActiveDeployments struct
	deployments := make([]PrintableActiveDeployments, numDeployments)
	for i, d := range response.Deployments {
		deployments[i] = PrintableActiveDeployments{
			Provider:    d.Provider.String(),
			ProjectName: d.Project,
			Region:      d.Region,
		}
	}

	// sort by provider then project name
	sort.Slice(deployments, func(i, j int) bool {
		if deployments[i].Provider == deployments[j].Provider {
			return deployments[i].ProjectName < deployments[j].ProjectName
		}
		return deployments[i].Provider < deployments[j].Provider
	})

	return term.Table(deployments, []string{"Provider", "ProjectName", "Region"})
}
