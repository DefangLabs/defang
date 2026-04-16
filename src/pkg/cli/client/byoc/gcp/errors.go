package gcp

import (
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/http"
	"google.golang.org/api/googleapi"
)

// The default googleapi.Error is too verbose, only display the message if it exists
type briefGcpError struct {
	err *googleapi.Error
}

func (e briefGcpError) Error() string {
	if e.err.Message != "" {
		return e.err.Message
	}
	return e.err.Error()
}

func (e briefGcpError) Unwrap() error {
	return e.err
}

func annotateGcpError(err error) error {
	gerr := new(googleapi.Error)
	if errors.As(err, &gerr) {
		briefErr := briefGcpError{err: gerr}
		// Check for forbidden errors to provide more context for ADC errors #1519
		if gerr.Code == http.StatusForbidden {
			for _, e := range gerr.Errors {
				if e.Reason == "forbidden" {
					return fmt.Errorf("double check the GCP project ID and make sure your Application Default Credentials have permission to access the project: %w", briefErr)
				}
			}
		}
		return briefErr
	}
	return err
}
