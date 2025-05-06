package cli

import (
	"context"
	"sort"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type PrintDeployment struct {
	Deployment  string
	DeployedAt  string
	ProjectName string
	Provider    string
	Region      string
}

func DeploymentsList(ctx context.Context, listType defangv1.DeploymentListType, projectName string, client client.GrpcClient, limit uint32) error {
	response, err := client.ListDeployments(ctx, &defangv1.ListDeploymentsRequest{
		Type:    listType,
		Project: projectName,
		Limit:   limit,
	})
	if err != nil {
		return err
	}

	numDeployments := len(response.Deployments)
	if numDeployments == 0 {
		var err error
		if projectName == "" {
			_, err = term.Warn("No deployments found")
		} else {
			_, err = term.Warnf("No deployments found for project %q", projectName)
		}
		return err
	}

	// map to Deployment struct
	deployments := make([]PrintDeployment, numDeployments)
	for i, d := range response.Deployments {
		deployments[i] = PrintDeployment{
			Deployment:  d.Id,
			DeployedAt:  d.Timestamp.AsTime().Format(time.RFC3339),
			ProjectName: d.Project,
			Provider:    d.Provider.String(), // TODO: use Provider
			Region:      d.Region,
		}
	}

	// sort by provider then project name
	sort.SliceStable(deployments, func(i, j int) bool {
		if deployments[i].Provider == deployments[j].Provider {
			return deployments[i].ProjectName < deployments[j].ProjectName
		}
		return deployments[i].Provider < deployments[j].Provider
	})

	return term.Table(deployments, []string{"Deployment", "Provider", "Region", "ProjectName", "DeployedAt"})
}
