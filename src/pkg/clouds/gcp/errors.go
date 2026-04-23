package gcp

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func IsNotFound(err error) bool {
	if grpcErr, ok := status.FromError(err); ok {
		if grpcErr.Code() == codes.NotFound {
			return true
		}
		if grpcErr.Code() == codes.Unknown && strings.HasSuffix(grpcErr.Message(), "notFound") {
			return true
		}
	}
	return false
}

func IsAccessNotEnabled(err error) bool {
	reasons := []string{
		"accessNotConfigured",
		"SERVICE_DISABLED",
	}

	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		for _, e := range gerr.Errors {
			if slices.Contains(reasons, e.Reason) {
				return true
			}
		}
	}
	var apiErr *apierror.APIError
	if errors.As(err, &apiErr) {
		if slices.Contains(reasons, apiErr.Reason()) {
			return true
		}
	}
	return false
}

// RetryOnAccessNotEnabled retries op up to attempts times, sleeping interval between attempts, while
// op returns an IsAccessNotEnabled error. Used after EnsureAPIsEnabled to tolerate the delay between
// an API enablement being returned as successful and the API actually being usable on subsequent calls.
func RetryOnAccessNotEnabled(ctx context.Context, attempts int, interval time.Duration, op func() error) error {
	var err error
	for i := range attempts {
		err = op()
		if err == nil || !IsAccessNotEnabled(err) {
			return err
		}
		if i < attempts-1 {
			term.Debugf("API not yet usable, will retry in %v: %v\n", interval, err)
			if sleepErr := pkg.SleepWithContext(ctx, interval); sleepErr != nil {
				return sleepErr
			}
		}
	}
	return err
}
