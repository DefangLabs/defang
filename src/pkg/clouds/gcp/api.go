package gcp

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
	"google.golang.org/api/serviceusage/v1"
)

func (gcp Gcp) EnsureAPIsEnabled(ctx context.Context, apis ...string) error {
	service, err := serviceusage.NewService(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Service Usage client: %w", err)
	}

	projectName := "projects/" + gcp.ProjectId

	listReq := service.Services.List(projectName).Filter("state:ENABLED")
	err = listReq.Pages(ctx, func(page *serviceusage.ListServicesResponse) error {
		for _, svc := range page.Services {
			if i := slices.Index(apis, svc.Config.Name); i != -1 {
				apis = slices.Delete(apis, i, i+1)
			}
		}
		return nil
	})
	if err != nil { // Ignore service usage API not being used
		return fmt.Errorf("failed to list enabled services: %w", err)
	}

	if len(apis) == 0 {
		return nil
	}

	term.Infof("Enabling services: %v\n", apis)
	req := &serviceusage.BatchEnableServicesRequest{
		ServiceIds: apis,
	}

	operation, err := service.Services.BatchEnable(projectName, req).Do()
	if err != nil {
		return fmt.Errorf("failed to batch enable services: %w", err)
	}

	opService := serviceusage.NewOperationsService(service)
	for {
		op, err := opService.Get(operation.Name).Do()
		if err != nil {
			return fmt.Errorf("failed to get operation status: %w", err)
		}

		// Check if the operation is done
		if op.Done {
			if op.Error != nil {
				return fmt.Errorf("error in operation: %v", op.Error)
			}
			return nil
		}
		pkg.SleepWithContext(ctx, 3*time.Second)
	}
}
