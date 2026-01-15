package gcp

import (
	"context"
	"errors"
	"fmt"
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

	for i := range 3 {
		term.Debugf("Enabling services: %v\n", apis)
		req := &serviceusage.BatchEnableServicesRequest{
			ServiceIds: apis,
		}

		operation, err := service.Services.BatchEnable(projectName, req).Context(ctx).Do()
		if err != nil {
			if i < 2 {
				term.Debugf("Failed to enable services, will retry in 5s: %v\n", err)
				pkg.SleepWithContext(ctx, 5*time.Second)
				continue
			}
			return fmt.Errorf("failed to batch enable services: %w", err)
		}

		opService := serviceusage.NewOperationsService(service)
		for {
			op, err := opService.Get(operation.Name).Context(ctx).Do()
			if err != nil {
				term.Warnf("Failed to get operation status: %v\n", err)
			} else if op.Done { // Check if the operation is done
				if op.Error != nil {
					if i < 2 {
						term.Debugf("Failed to enable services operation, will retry in 5s: %v\n", op.Error)
						pkg.SleepWithContext(ctx, 5*time.Second)
						break
					}
					return fmt.Errorf("error in operation: %v", op.Error)
				}
				return nil
			}
			pkg.SleepWithContext(ctx, 3*time.Second)
		}
	}
	return errors.New("failed to enable services after 3 attempts") // This should never be reached
}
