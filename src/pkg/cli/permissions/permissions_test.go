package permissions

import (
	"strings"
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestPermission(t *testing.T) {
	test := []struct {
		name              string
		action            ActionRequest
		expectedResult    bool
		expectedErrorText string
	}{
		{
			name: "no permission for gpu",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PERSONAL,
				action:   "compose-up",
				resource: "gpu",
			},
			expectedErrorText: "gpus not supported by this tier",
		},
		{
			name: "have permission for gpu",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PRO,
				action:   "compose-up",
				resource: "gpu",
				detail:   "1",
			},
			expectedErrorText: "",
		},
		{
			name: "have permission gpu 0",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_BASIC,
				action:   "compose-up",
				resource: "gpu",
				detail:   "0",
			},
			expectedErrorText: "",
		},
		{
			name: "no permission gpu 1",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_BASIC,
				action:   "compose-up",
				resource: "gpu",
				detail:   "1",
			},
			expectedResult:    false,
			expectedErrorText: "gpus not supported by this tier",
		},
		{
			name: "has permission provider aws",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PERSONAL,
				action:   "compose-up",
				resource: "aws",
			},
			expectedErrorText: "deploy to aws",
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
			err := HasPermission(tt.action.tier, tt.action.action, tt.action.resource, tt.action.detail, tt.expectedErrorText)
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
