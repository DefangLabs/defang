package deployment_info

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ErrNoServices struct {
	ProjectName string // may be empty
}

func (e ErrNoServices) Error() string {
	return fmt.Sprintf("no services found in project %q", e.ProjectName)
}

type Service struct {
	Service      string
	DeploymentId string
	PublicFqdn   string
	PrivateFqdn  string
	Status       string
}

func GetServices(ctx context.Context, projectName string, provider client.Provider) ([]Service, error) {
	term.Infof("Listing services in project %q", projectName)

	term.Info("Function invoked: provider.GetServices")
	getServicesResponse, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName})
	if err != nil {
		return nil, err
	}

	numServices := len(getServicesResponse.Services)

	if numServices == 0 {
		return nil, ErrNoServices{ProjectName: projectName}
	}

	result := make([]Service, numServices)
	for i, si := range getServicesResponse.Services {
		result[i] = Service{
			Service:      si.Service.Name,
			DeploymentId: si.Etag,
			PublicFqdn:   si.PublicFqdn,
			PrivateFqdn:  si.PrivateFqdn,
			Status:       si.Status,
		}
	}

	return result, nil
}
