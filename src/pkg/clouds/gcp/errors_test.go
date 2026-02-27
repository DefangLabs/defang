package gcp

import (
	"testing"

	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/api/googleapi"
)

func TestIsAccessNotConfigured(t *testing.T) {
	body := `{
  "error": {
    "code": 403,
    "message": "Cloud DNS API has not been used in project notional-fusion-485119-t4 before or it is disabled. Enable it by visiting https://console.developers.google.com/apis/api/dns.googleapis.com/overview?project=notional-fusion-485119-t4 then retry. If you enabled this API recently, wait a few minutes for the action to propagate to our systems and retry.",
    "errors": [
      {
        "message": "Cloud DNS API has not been used in project notional-fusion-485119-t4 before or it is disabled. Enable it by visiting https://console.developers.google.com/apis/api/dns.googleapis.com/overview?project=notional-fusion-485119-t4 then retry. If you enabled this API recently, wait a few minutes for the action to propagate to our systems and retry.",
        "domain": "usageLimits",
        "reason": "accessNotConfigured",
        "extendedHelp": "https://console.developers.google.com"
      }
    ],
    "status": "PERMISSION_DENIED",
    "details": [
      {
        "@type": "type.googleapis.com/google.rpc.ErrorInfo",
        "reason": "SERVICE_DISABLED",
        "domain": "googleapis.com",
        "metadata": {
          "activationUrl": "https://console.developers.google.com/apis/api/dns.googleapis.com/overview?project=notional-fusion-485119-t4",
          "consumer": "projects/notional-fusion-485119-t4",
          "containerInfo": "notional-fusion-485119-t4",
          "serviceTitle": "Cloud DNS API",
          "service": "dns.googleapis.com"
        }
      },
      {
        "@type": "type.googleapis.com/google.rpc.LocalizedMessage",
        "locale": "en-US",
        "message": "Cloud DNS API has not been used in project notional-fusion-485119-t4 before or it is disabled. Enable it by visiting https://console.developers.google.com/apis/api/dns.googleapis.com/overview?project=notional-fusion-485119-t4 then retry. If you enabled this API recently, wait a few minutes for the action to propagate to our systems and retry."
      },
      {
        "@type": "type.googleapis.com/google.rpc.Help",
        "links": [
          {
            "description": "Google developers console API activation",
            "url": "https://console.developers.google.com/apis/api/dns.googleapis.com/overview?project=notional-fusion-485119-t4"
          }
        ]
      }
    ]
  }
}
`
	err := &googleapi.Error{
		Code:    403,
		Message: "Cloud DNS API has not been used in project notional-fusion-485119-t4 before or it is disabled. Enable it by visiting https://console.developers.google.com/apis/api/dns.googleapis.com/overview?project=notional-fusion-485119-t4 then retry. If you enabled this API recently, wait a few minutes for the action to propagate to our systems and retry.",
		Body:    body,
		Errors: []googleapi.ErrorItem{
			{
				Message: "Cloud DNS API has not been used in project notional-fusion-485119-t4 before or it is disabled. Enable it by visiting https://console.developers.google.com/apis/api/dns.googleapis.com/overview?project=notional-fusion-485119-t4 then retry. If you enabled this API recently, wait a few minutes for the action to propagate to our systems and retry.",
				Reason:  "accessNotConfigured",
			},
		},
	}

	if !IsAccessNotEnabled(err) {
		t.Errorf("expected IsAccessNotEnabled to return true, got false")
	}
}

func TestServiceDisabled(t *testing.T) {
	body := `{
  "error": {
    "code": 403,
    "message": "Cloud DNS API has not been used in project notional-fusion-485119-t4 before or it is disabled. Enable it by visiting https://console.developers.google.com/apis/api/dns.googleapis.com/overview?project=notional-fusion-485119-t4 then retry. If you enabled this API recently, wait a few minutes for the action to propagate to our systems and retry.",
    "status": "PERMISSION_DENIED",
    "details": [
      {
        "@type": "type.googleapis.com/google.rpc.ErrorInfo",
        "reason": "SERVICE_DISABLED",
        "domain": "googleapis.com",
        "metadata": {
          "activationUrl": "https://console.developers.google.com/apis/api/dns.googleapis.com/overview?project=notional-fusion-485119-t4",
          "consumer": "projects/notional-fusion-485119-t4",
          "containerInfo": "notional-fusion-485119-t4",
          "serviceTitle": "Cloud DNS API",
          "service": "dns.googleapis.com"
        }
      }
    ]
  }
}`

	err := &googleapi.Error{
		Code:    403,
		Message: "Cloud DNS API has not been used in project notional-fusion-485119-t4 before or it is disabled. Enable it by visiting https://console.developers.google.com/apis/api/dns.googleapis.com/overview?project=notional-fusion-485119-t4 then retry. If you enabled this API recently, wait a few minutes for the action to propagate to our systems and retry.",
		Body:    body,
	}
	apierr, _ := apierror.ParseError(err, true)

	if !IsAccessNotEnabled(apierr) {
		t.Errorf("expected IsAccessNotEnabled to return true, got false")
	}
}
