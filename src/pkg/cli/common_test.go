package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type mockFabricForCommon struct {
	client.MockFabricClient
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

func TestPutDeploymentAndStack_ContinuesOnPutStackError(t *testing.T) {
	fabric := &mockFabricForCommon{
		putStackErr: errors.New("PutStack failed"),
	}
	provider := client.MockProvider{}

	err := putDeploymentAndStack(context.Background(), provider, fabric, nil, putDeploymentParams{
		Action:      defangv1.DeploymentAction_DEPLOYMENT_ACTION_UP,
		ProjectName: "test-project",
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !fabric.putDeploymentCalled {
		t.Error("PutDeployment should be called even when PutStack fails")
	}
}

func TestPutDeploymentAndStack_CallsDeploymentOnSuccess(t *testing.T) {
	fabric := &mockFabricForCommon{}
	provider := client.MockProvider{}

	err := putDeploymentAndStack(context.Background(), provider, fabric, nil, putDeploymentParams{
		Action:      defangv1.DeploymentAction_DEPLOYMENT_ACTION_UP,
		ProjectName: "test-project",
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !fabric.putDeploymentCalled {
		t.Error("PutDeployment should be called when PutStack succeeds")
	}
}
