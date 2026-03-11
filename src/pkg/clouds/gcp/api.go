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

// retryWithContext calls op up to maxRetries times, sleeping retryInterval between attempts.
// It returns the last error if all attempts fail.
func retryWithContext(ctx context.Context, op func() error) error {
	var err error
	for i := range maxRetries {
		if err = op(); err == nil {
			return nil
		}
		if i < maxRetries-1 {
			term.Debugf("Operation failed, will retry in %v: %v\n", retryInterval, err)
			if sleepErr := pkg.SleepWithContext(ctx, retryInterval); sleepErr != nil {
				return sleepErr
			}
		}
	}
	return err
}

func (gcp Gcp) EnsureAPIsEnabled(ctx context.Context, apis ...string) error {
	service, err := serviceusage.NewService(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Service Usage client: %w", err)
	}

	projectName := "projects/" + gcp.ProjectId

	return retryWithContext(ctx, func() error {
		term.Debugf("Enabling services: %v\n", apis)
		req := &serviceusage.BatchEnableServicesRequest{ServiceIds: apis}
		operation, err := service.Services.BatchEnable(projectName, req).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("failed to batch enable services: %w", err)
		}

		opService := serviceusage.NewOperationsService(service)
		for {
			op, err := opService.Get(operation.Name).Context(ctx).Do()
			if err != nil {
				term.Warnf("Failed to get operation status: %v\n", err)
			} else if op.Done {
				if op.Error != nil {
					return fmt.Errorf("enable services operation failed: %v", op.Error)
				}
				return nil
			}
			if err := pkg.SleepWithContext(ctx, 3*time.Second); err != nil {
				return err
			}
		}
	})
}
