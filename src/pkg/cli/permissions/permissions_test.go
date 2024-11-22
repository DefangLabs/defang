package permissions

import (
	"errors"
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestPermission(t *testing.T) {

	test := []struct {
		name           string
		action         ActionRequest
		expectedResult bool
		expectedError  error
	}{
		{
			name: "no permission for gpu",
			action: ActionRequest{
				role:     defangv1.SubscriptionTier_PERSONAL,
				action:   "deploy",
				resource: "gpu",
			},
			expectedResult: false,
			expectedError:  nil,
		},
		{
			name: "have permission for gpu",
			action: ActionRequest{
				role:     defangv1.SubscriptionTier_PRO,
				action:   "deploy",
				resource: "gpu",
				detail:   "1",
			},
			expectedResult: true,
			expectedError:  nil,
		},
		{
			name: "have permission gpu 0",
			action: ActionRequest{
				role:     defangv1.SubscriptionTier_BASIC,
				action:   "deploy",
				resource: "gpu",
				detail:   "0",
			},
			expectedResult: true,
			expectedError:  nil,
		},
		{
			name: "no permission gpu 1",
			action: ActionRequest{
				role:     defangv1.SubscriptionTier_BASIC,
				action:   "deploy",
				resource: "gpu",
				detail:   "1",
			},
			expectedResult: false,
			expectedError:  nil,
		},
		{
			name: "has permission provider aws",
			action: ActionRequest{
				role:     defangv1.SubscriptionTier_PERSONAL,
				action:   "deploy",
				resource: "aws",
			},
			expectedResult: false,
			expectedError:  nil,
		},
		{
			name: "unknown permission check errors",
			action: ActionRequest{
				role:     defangv1.SubscriptionTier_PERSONAL,
				action:   "do",
				resource: "random",
			},
			expectedResult: false,
			expectedError:  errors.New("unknown resource: random"),
		},
	}

	for _, tt := range test {
		t.Run(tt.name, func(t *testing.T) {
			hasPermission, err := HasPermission(tt.action.role, tt.action.action, tt.action.resource, tt.action.detail)
			if tt.expectedError == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.expectedError != nil && err == nil {
				t.Fatal("expected error not found")
			}

			if tt.expectedError != nil && err != nil && tt.expectedError.Error() != err.Error() {
				t.Fatalf("expected error %s but got %s", tt.expectedError.Error(), err.Error())
			}

			if hasPermission != tt.expectedResult {
				t.Errorf("expected permission to be %t got %t", tt.expectedResult, hasPermission)
			}
		})
	}
}
