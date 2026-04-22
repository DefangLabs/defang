package cli

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type mockFabricForCommon struct {
	client.FabricClient
	putStackErr         error
	putDeploymentCalled bool
}

func (m *mockFabricForCommon) PutStack(_ context.Context, _ *defangv1.PutStackRequest) error {
	return m.putStackErr
}

func (m *mockFabricForCommon) PutDeployment(_ context.Context, _ *defangv1.PutDeploymentRequest) error {
	m.putDeploymentCalled = true
	return nil
}

func TestPutDeploymentAndStack(t *testing.T) {
	tests := []struct {
		name              string
		putStackErr       error
		wantErr           bool
		wantPutDeployment bool
	}{
		{
			name:              "PutStack AlreadyExists, but PutDeployment is still called",
			putStackErr:       connect.NewError(connect.CodeAlreadyExists, errors.New("stack already exists")),
			wantErr:           false,
			wantPutDeployment: true,
		},
		{
			name:              "PutStack failed, PutDeployment errors",
			putStackErr:       errors.New("PutStack failed"),
			wantErr:           true,
			wantPutDeployment: false,
		},
		{
			name:              "PutStack succeeds, and PutDeployment is called",
			putStackErr:       nil,
			wantErr:           false,
			wantPutDeployment: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fabric := &mockFabricForCommon{
				putStackErr: tt.putStackErr,
			}
			provider := client.MockProvider{}

			err := putDeploymentAndStack(context.Background(), provider, fabric, nil, putDeploymentParams{
				Action:      defangv1.DeploymentAction_DEPLOYMENT_ACTION_UP,
				ProjectName: "test-project",
			})

			if err != nil {
				if !tt.wantErr {
					t.Fatalf("expected no error, got: %v", err)
				}
			} else {
				if tt.wantErr {
					t.Fatalf("expected error, got nil")
				}
			}
			if fabric.putDeploymentCalled != tt.wantPutDeployment {
				t.Errorf("expected PutDeploymentCalled to be %v, got %v", tt.wantPutDeployment, fabric.putDeploymentCalled)
			}
		})
	}
}
