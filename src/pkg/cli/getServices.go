package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ErrNoServices struct {
	ProjectName string // may be empty
}

func (e ErrNoServices) Error() string {
	return "no services found in project " + e.ProjectName // no quotes because ProjectName may be empty
}

func GetServices(ctx context.Context, projectName string, provider client.Provider, long bool) error {
	term.Debugf("Listing services in project %q", projectName)

	servicesResponse, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName})
	if err != nil {
		return err
	}

	numServices := len(servicesResponse.Services)
	if numServices == 0 {
		return ErrNoServices{ProjectName: projectName}
	}

	if long {
		return PrintObject("", servicesResponse)
	}

	return PrintServiceStatesAndEndpoints(ctx, servicesResponse.Services)
}
