package gcp

import (
	"errors"
	"net/http"
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
	var gerr *googleapi.Error
	if errors.As(err, &gerr) && gerr.Code == http.StatusForbidden {
		for _, e := range gerr.Errors {
			if e.Reason == "accessNotConfigured" {
				return true
			}
			if e.Reason == "SERVICE_DISABLED" {
				return true
			}
		}
	}
	var apiErr *apierror.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Reason() == "SERVICE_DISABLED" {
			return true
		}

		if apiErr.Reason() == "accessNotConfigured" {
			return true
		}
	}
	return false
}
