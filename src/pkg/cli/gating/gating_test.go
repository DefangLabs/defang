package gating

import (
	"strings"
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestAccessGates(t *testing.T) {
	test := []struct {
		name              string
		action            ActionRequest
		expectedResult    bool
		expectedErrorText string
	}{
		{
			name: "no permission for provider = aws",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_HOBBY,
				action:   "use-provider",
				resource: "aws",
			},
			expectedErrorText: "aws not supported by this tier",
		},
		{
			name: "has permission for provider = aws",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PERSONAL,
				action:   "use-provider",
				resource: "aws",
			},
			expectedErrorText: "",
		},
		{
			name: "no permission for gpu",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PERSONAL,
				action:   "use-gpu",
				resource: "gpu",
			},
			expectedErrorText: "gpus not supported by this tier",
		},
		{
			name: "have permission for gpu",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PRO,
				action:   "use-gpu",
				resource: "gpu",
				count:    1,
			},
			expectedErrorText: "",
		},
		{
			name: "have permission gpu 0",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PERSONAL,
				action:   "use-gpu",
				resource: "gpu",
				count:    0,
			},
			expectedErrorText: "",
		},
		{
			name: "no permission gpu 1",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PERSONAL,
				action:   "use-gpu",
				resource: "gpu",
				count:    1,
			},
			expectedResult:    false,
			expectedErrorText: "gpus not supported by this tier",
		},
		{
			name: "have permission managed postgres",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PRO,
				action:   "use-managed",
				resource: "postgres",
				count:    1,
			},
			expectedErrorText: "",
		},
		{
			name: "have permission managed redis",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PRO,
				action:   "use-managed",
				resource: "redis",
				count:    1,
			},
			expectedErrorText: "",
		},
		{
			name: "unknown permission check errors",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PERSONAL,
				action:   "do",
				resource: "random",
			},
			expectedErrorText: "unknown resource: random",
		},
	}

	for _, tt := range test {
		t.Run(tt.name, func(t *testing.T) {
			err := HasAuthorization(tt.action.tier, tt.action.action, tt.action.resource, tt.action.count, tt.expectedErrorText)
			if err != nil {
				if !strings.Contains(err.Error(), tt.expectedErrorText) {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if err == nil && tt.expectedErrorText != "" {
				t.Fatal("expected error but not found")
			}
		})
	}
}
