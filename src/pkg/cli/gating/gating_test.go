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
				action:   ActionUseProvider,
				resource: ResourceAWS,
			},
			expectedErrorText: "aws not supported by this tier",
		},
		{
			name: "has permission for provider = aws",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PERSONAL,
				action:   ActionUseProvider,
				resource: ResourceAWS,
			},
			expectedErrorText: "",
		},
		{
			name: "no permission for gpu",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PERSONAL,
				action:   ActionUseGPU,
				resource: ResourceGPU,
			},
			expectedErrorText: "gpus not supported by this tier",
		},
		{
			name: "have permission for gpu",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PRO,
				action:   ActionUseGPU,
				resource: ResourceGPU,
			},
			expectedErrorText: "",
		},
		{
			name: "have permission managed postgres",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PRO,
				action:   ActionUseManaged,
				resource: ResourcePostgres,
			},
			expectedErrorText: "",
		},
		{
			name: "have permission managed redis",
			action: ActionRequest{
				tier:     defangv1.SubscriptionTier_PRO,
				action:   ActionUseManaged,
				resource: ResourceRedis,
			},
			expectedErrorText: "",
		},
	}

	for _, tt := range test {
		t.Run(tt.name, func(t *testing.T) {
			err := HasAuthorization(tt.action.tier, tt.action.action, string(tt.action.resource), tt.expectedErrorText)
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
