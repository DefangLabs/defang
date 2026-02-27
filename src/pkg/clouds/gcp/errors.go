package gcp

import (
	"errors"
	"slices"
	"strings"

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
