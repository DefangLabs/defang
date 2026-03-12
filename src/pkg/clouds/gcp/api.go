package gcp

import (
	"context"
	"fmt"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
	"google.golang.org/api/serviceusage/v1"
)

const (
	// we have seen it take up to 3 minutes to enable APIs and create new service accounts
	maxRetries    = 36
	retryInterval = 5 * time.Second
)

func (gcp Gcp) EnsureAPIsEnabled(ctx context.Context, apis ...string) error {
	service, err := serviceusage.NewService(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Service Usage client: %w", err)
	}

	projectName := "projects/" + gcp.ProjectId

	for i := range maxRetries {
		term.Debugf("Enabling services: %v\n", apis)
		req := &serviceusage.BatchEnableServicesRequest{
			ServiceIds: apis,
		}

		operation, err := service.Services.BatchEnable(projectName, req).Context(ctx).Do()
		if err != nil {
			if i < maxRetries-1 {
				term.Debugf("Failed to enable services, will retry in %v: %v\n", retryInterval, err)
				if err := pkg.SleepWithContext(ctx, retryInterval); err != nil {
					return err
				}
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
					if i < maxRetries-1 {
						term.Debugf("Failed to enable services operation, will retry in %v: %v\n", retryInterval, op.Error)
						if err := pkg.SleepWithContext(ctx, retryInterval); err != nil {
							return err
						}
						continue
					}
					return fmt.Errorf("error in operation: %v", op.Error)
				}
				return nil
			}
			if err := pkg.SleepWithContext(ctx, 3*time.Second); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("failed to enable services after %d retries", maxRetries) // This should never be reached
}
