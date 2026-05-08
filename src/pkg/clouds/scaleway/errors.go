package scaleway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// APIError represents an error returned by the Scaleway API.
type APIError struct {
	StatusCode int    `json:"-"`
	Message    string `json:"message"`
	Type       string `json:"type"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("scaleway: %s (HTTP %d)", e.Message, e.StatusCode)
}

// parseAPIError reads an error response body and returns a structured APIError.
func parseAPIError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("scaleway: HTTP %d (failed to read body: %w)", resp.StatusCode, err)
	}

	apiErr := &APIError{StatusCode: resp.StatusCode}
	if err := json.Unmarshal(body, apiErr); err != nil || apiErr.Message == "" {
		apiErr.Message = string(body)
	}
	return apiErr
}

// IsNotFound returns true if the error is a 404 Not Found response.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// IsConflict returns true if the error is a 409 Conflict response.
func IsConflict(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict
}

// AnnotateScalewayError wraps a Scaleway API error with user-friendly context.
func AnnotateScalewayError(err error, action string) error {
	if err == nil {
		return nil
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("%s: %w", action, err)
	}
	switch apiErr.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%s: invalid Scaleway credentials — verify SCW_ACCESS_KEY and SCW_SECRET_KEY (https://www.scaleway.com/en/docs/identity-and-access-management/iam/how-to/create-api-keys/): %w", action, err)
	case http.StatusForbidden:
		return fmt.Errorf("%s: insufficient permissions — check your Scaleway IAM policy: %w", action, err)
	case http.StatusNotFound:
		return fmt.Errorf("%s: resource not found: %w", action, err)
	case http.StatusConflict:
		return fmt.Errorf("%s: resource already exists: %w", action, err)
	default:
		return fmt.Errorf("%s: %w", action, err)
	}
}
