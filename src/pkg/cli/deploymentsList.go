package cli

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type PrintDeployment struct {
	AccountId   string
	Deployment  string
	DeployedAt  string
	ProjectName string
	Provider    string
	Region      string
}

func DeploymentsList(ctx context.Context, listType defangv1.DeploymentType, projectName string, client client.FabricClient, limit uint32) error {
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
			AccountId:   d.ProviderAccountId,
			DeployedAt:  d.Timestamp.AsTime().Local().Format(time.RFC3339),
			Deployment:  d.Id,
			ProjectName: d.Project,
			Provider:    getProvider(d.Provider, d.ProviderString),
			Region:      d.Region,
		}
	}

	// sort by project name, provider, account id, and region
	sortKeys := make([]string, numDeployments)
	for i, d := range deployments {
		// TODO: allow user to specify sort order
		sortKeys[i] = strings.Join([]string{d.ProjectName, d.Provider, d.AccountId, d.Region}, "|")
	}
	slices.SortStableFunc(sortKeys, func(a, b string) int {
		return strings.Compare(a, b)
	})

	return term.Table(deployments, []string{"ProjectName", "Provider", "AccountId", "Region", "Deployment", "DeployedAt"})
}

func getProvider(provider defangv1.Provider, providerString string) string {
	if provider == defangv1.Provider_PROVIDER_UNSPECIFIED {
		return providerString
	}
	return strings.ToLower(provider.String())
}
