package gcp

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/api/googleapi"
)

func Test_BadGCPprojectnameErrorWrap(t *testing.T) {
	// Raw test data from GCP API error response
	rawBody := `{
  "error": {
    "code": 403,
    "message": "Project 'badprojectname' not found or permission denied.\nHelp Token: AcxmRmLSl-kWjuntWxEi67mxGpzJGGd2wYETCeD62ggFgHBNMRS8zqDD7iOMq9vTMPRs1XyX8q0G4JfCDJfKvTaPSV40gsoBHUJrq_eVdmFXUpNT",
    "errors": [
      {
        "message": "Project 'badprojectname' not found or permission denied.\nHelp Token: AcxmRmLSl-kWjuntWxEi67mxGpzJGGd2wYETCeD62ggFgHBNMRS8zqDD7iOMq9vTMPRs1XyX8q0G4JfCDJfKvTaPSV40gsoBHUJrq_eVdmFXUpNT",
        "domain": "global",
        "reason": "forbidden"
      }
    ],
    "status": "PERMISSION_DENIED",
    "details": [
      {
        "@type": "type.googleapis.com/google.rpc.PreconditionFailure",
        "violations": [
          {
            "type": "googleapis.com",
            "subject": "?error_code=210002&type=Project&resource_id=badprojectname"
          }
        ]
      },
      {
        "@type": "type.googleapis.com/google.rpc.ErrorInfo",
        "reason": "RESOURCES_NOT_FOUND",
        "domain": "serviceusage.googleapis.com",
        "metadata": {
          "type": "Project",
          "resource_id": "badprojectname"
        }
      }
    ]
  }
}`

	gcpErr := &googleapi.Error{
		Code:    403,
		Message: "Project 'badprojectname' not found or permission denied.\nHelp Token: AcxmRmLSl-kWjuntWxEi67mxGpzJGGd2wYETCeD62ggFgHBNMRS8zqDD7iOMq9vTMPRs1XyX8q0G4JfCDJfKvTaPSV40gsoBHUJrq_eVdmFXUpNT",
		Body:    rawBody,
	}

	briefErr := briefGcpError{err: gcpErr}

	// Test Error() returns custom message with project name
	errMsg := briefErr.Error()
	assert.Equal(t, errMsg, gcpErr.Message)

	// Test annotateGcpError wraps the error correctly
	wrappedErr := annotateGcpError(gcpErr)
	assert.Equal(t, `double check the GCP project ID and make sure your Application Default Credentials have permission to access the project: `+gcpErr.Message, wrappedErr.Error())

	// Test the error is wrapper correctly
	var unwrappedErr *googleapi.Error
	assert.True(t, errors.As(wrappedErr, &unwrappedErr))
	assert.Equal(t, gcpErr, unwrappedErr)
}
