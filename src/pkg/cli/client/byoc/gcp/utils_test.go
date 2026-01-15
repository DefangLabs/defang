package gcp

import (
	"testing"

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

	// Test extractProjectName
	projectName := briefErr.extractProjectName()
	if projectName != "badprojectname" {
		t.Errorf("extractProjectName() = %q, want %q", projectName, "badprojectname")
	}

	// Test Error() returns custom message with project name
	errMsg := briefErr.Error()
	expectedMsg := `GCP project "badprojectname" not found or permission denied. ` +
		`Double check the project ID and make sure your Application Default Credentials ` +
		`have permission to access the project.`

	if errMsg != expectedMsg {
		t.Errorf("Error() = %q, want %q", errMsg, expectedMsg)
	}
}
