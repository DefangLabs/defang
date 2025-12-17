package cli

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func TestPreviewStops(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow unit test")
	}

	fabric := client.MockFabricClient{DelegateDomain: "example.com"}
	project := &compose.Project{
		Name: "test-project",
		Services: compose.Services{
			"service1": compose.ServiceConfig{
				Name:       "service1",
				Image:      "test-image",
				DomainName: "test-domain",
			},
		},
	}

	tests := []struct {
		name      string
		err       error
		wantError string
	}{
		{"CD task fails", ecs.TaskFailure{Reason: types.TaskStopCodeEssentialContainerExited, Detail: "exit code 1"}, "EssentialContainerExited: exit code 1"},
		{"CD task succeeds", io.EOF, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockDeployProvider{
				deploymentStatus: tt.err,
			}

			err := Preview(t.Context(), project, fabric, provider, ComposeUpParams{Mode: modes.ModeUnspecified, Project: project})
			if err != nil {
				if err.Error() != tt.wantError {
					t.Errorf("got error: %v, want: %v", err, tt.wantError)
				}
			} else if tt.wantError != "" {
				t.Errorf("expected error %v, got nil", tt.wantError)
			}
		})
	}

	t.Run("Context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(nil) // to cancel tail and clean-up context

		cancelErr := errors.New("custom error")
		time.AfterFunc(1*time.Second, func() {
			cancel(cancelErr) // this will close the ServerStream gracefully
		})

		provider := &mockDeployProvider{}

		err := Preview(ctx, project, fabric, provider, ComposeUpParams{Mode: modes.ModeUnspecified, Project: project})
		if err != nil {
			t.Errorf("got error: %v, want nil", err)
		}
	})
}
