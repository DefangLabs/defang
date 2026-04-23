package gcp

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestRetryOnAccessNotEnabled(t *testing.T) {
	accessNotEnabledErr := &googleapi.Error{
		Code: 403,
		Errors: []googleapi.ErrorItem{
			{Reason: "accessNotConfigured"},
		},
	}

	t.Run("returns nil on success without retry", func(t *testing.T) {
		calls := 0
		err := RetryOnAccessNotEnabled(t.Context(), 3, time.Millisecond, func() error {
			calls++
			return nil
		})
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("returns non-access-not-enabled error without retry", func(t *testing.T) {
		calls := 0
		other := errors.New("some other error")
		err := RetryOnAccessNotEnabled(t.Context(), 3, time.Millisecond, func() error {
			calls++
			return other
		})
		if !errors.Is(err, other) {
			t.Errorf("expected %v, got %v", other, err)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("retries on access-not-enabled then succeeds", func(t *testing.T) {
		calls := 0
		err := RetryOnAccessNotEnabled(t.Context(), 5, time.Millisecond, func() error {
			calls++
			if calls < 3 {
				return accessNotEnabledErr
			}
			return nil
		})
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
		if calls != 3 {
			t.Errorf("expected 3 calls, got %d", calls)
		}
	})

	t.Run("returns last error after exhausting attempts", func(t *testing.T) {
		calls := 0
		err := RetryOnAccessNotEnabled(t.Context(), 3, time.Millisecond, func() error {
			calls++
			return accessNotEnabledErr
		})
		if !errors.Is(err, accessNotEnabledErr) {
			t.Errorf("expected access-not-enabled error, got %v", err)
		}
		if calls != 3 {
			t.Errorf("expected 3 calls, got %d", calls)
		}
	})

	t.Run("returns context error when cancelled mid-retry", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		calls := 0
		err := RetryOnAccessNotEnabled(ctx, 5, 50*time.Millisecond, func() error {
			calls++
			cancel()
			return accessNotEnabledErr
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})
}
