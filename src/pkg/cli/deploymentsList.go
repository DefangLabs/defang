package cli

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type DeploymentLineItem struct {
	AccountId   string
	Deployment  string
	DeployedAt  string
	ProjectName string
	Stack       string
	Provider    string
	Region      string
	Mode        string
}

type ListDeploymentsParams struct {
	ListType    defangv1.DeploymentType
	ProjectName string
	StackName   string
	Limit       uint32
}

func DeploymentsList(ctx context.Context, client client.FabricClient, params ListDeploymentsParams) error {
	response, err := client.ListDeployments(ctx, &defangv1.ListDeploymentsRequest{
		Type:    params.ListType,
		Project: params.ProjectName,
		Stack:   params.StackName,
		Limit:   params.Limit,
	})
	if err != nil {
		return err
	}

	numDeployments := len(response.Deployments)
	if numDeployments == 0 {
		var err error
		if params.ProjectName == "" {
			_, err = term.Warn("No deployments found")
		} else {
			_, err = term.Warnf("No deployments found for project %q", params.ProjectName)
		}
		return err
	}

	// map to Deployment struct
	deployments := make([]DeploymentLineItem, numDeployments)
	for i, d := range response.Deployments {
		deployments[i] = DeploymentLineItem{
			AccountId:   d.ProviderAccountId,
			DeployedAt:  d.Timestamp.AsTime().Local().Format(time.RFC3339),
			Deployment:  d.Id,
			ProjectName: d.Project,
			Stack:       d.Stack,
			Provider:    getProvider(d.Provider, d.ProviderString),
			Region:      d.Region,
			Mode:        d.Mode.String(),
		}
	}

	// sort by project name, provider, account id, and region
	sortKeys := make([]string, numDeployments)
	for i, d := range deployments {
		// TODO: allow user to specify sort order
		sortKeys[i] = strings.Join([]string{d.ProjectName, d.Stack, d.Provider, d.AccountId, d.Region}, "|")
	}
	sort.SliceStable(sortKeys, func(i, j int) bool {
		return sortKeys[i] < sortKeys[j]
	})

	return term.Table(deployments, "ProjectName", "Stack", "Provider", "AccountId", "Region", "Deployment", "Mode", "DeployedAt")
}

func getProvider(provider defangv1.Provider, providerString string) string {
	if provider == defangv1.Provider_PROVIDER_UNSPECIFIED {
		return providerString
	}
	return strings.ToLower(provider.String())
}
