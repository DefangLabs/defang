package gcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/serviceusage/v1"
)

const (
	// we have seen it take up to 3 minutes to enable APIs and create new service accounts
	maxAttempts   = 36
	retryInterval = 5 * time.Second
)

func (gcp Gcp) EnsureAPIsEnabled(ctx context.Context, apis ...string) error {
	service, err := serviceusage.NewService(ctx, gcp.Options...)
	if err != nil {
		return fmt.Errorf("failed to create Service Usage client: %w", err)
	}

	projectName := "projects/" + gcp.ProjectId

	for i := range maxAttempts {
		slog.Debug(fmt.Sprintf("Enabling services: %v\n", apis))
		req := &serviceusage.BatchEnableServicesRequest{
			ServiceIds: apis,
		}

		operation, err := service.Services.BatchEnable(projectName, req).Context(ctx).Do()
		if err != nil {
			// Do not retry on permission errors
			var apiErr *googleapi.Error
			if errors.As(err, &apiErr) && (apiErr.Code == 403 || apiErr.Code == 401) {
				return fmt.Errorf("permission denied when enabling services: %w", err)
			}
			slog.ErrorContext(ctx, fmt.Sprintf("Error: %+v (%T)", err, err))
			if i < maxAttempts-1 {
				slog.Debug(fmt.Sprintf("Failed to enable services, will retry in %v: %v\n", retryInterval, err))
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
				slog.WarnContext(ctx, fmt.Sprintf("Failed to get operation status: %v\n", err))
			} else if op.Done { // Check if the operation is done
				if op.Error != nil {
					if i < maxAttempts-1 {
						slog.Debug(fmt.Sprintf("Failed to enable services operation, will retry in %v: %v\n", retryInterval, op.Error))
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
	return fmt.Errorf("failed to enable services after %d attempts", maxAttempts) // This should never be reached
}
