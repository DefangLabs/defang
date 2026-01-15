package gcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/http"
	"google.golang.org/api/googleapi"
)

type gcpErrorResponse struct {
	Error struct {
		Details []struct {
			Type     string `json:"@type"`
			Metadata struct {
				ResourceID string `json:"resource_id"`
			} `json:"metadata,omitempty"`
		} `json:"details"`
	} `json:"error"`
}

// Error string type found in invalid project errors from GCP APIs
const googleAPIErrorInfoType = "type.googleapis.com/google.rpc.ErrorInfo"

// The default googleapi.Error is too verbose, only display the message if it exists
type briefGcpError struct {
	err *googleapi.Error
}

func (e briefGcpError) extractProjectName() string {
	var errResp gcpErrorResponse
	if err := json.Unmarshal([]byte(e.err.Body), &errResp); err != nil {
		return ""
	}

	for _, detail := range errResp.Error.Details {
		if detail.Type == googleAPIErrorInfoType {
			return detail.Metadata.ResourceID
		}
	}
	return ""
}

func (e briefGcpError) Error() string {
	if e.err.Message != "" {
		if e.err.Code == http.StatusForbidden {
			projectName := e.extractProjectName()
			if projectName != "" {
				return fmt.Sprintf("GCP project %q not found or permission denied. Double check the project ID and make sure your Application Default Credentials have permission to access the project.", projectName)
			}
		}
		return e.err.Message
	}
	return e.err.Error()
}

func annotateGcpError(err error) error {
	gerr := new(googleapi.Error)
	if errors.As(err, &gerr) {
		return briefGcpError{err: gerr}
	}
	return err
}

// Used to get nested values from the detail of a googleapi.Error
func GetGoogleAPIErrorDetail(detail any, path string) string {
	if path == "" {
		value, ok := detail.(string)
		if ok {
			return value
		}
		return ""
	}
	dm, ok := detail.(map[string]any)
	if !ok {
		return ""
	}
	key, rest, _ := strings.Cut(path, ".")
	return GetGoogleAPIErrorDetail(dm[key], rest)
}
