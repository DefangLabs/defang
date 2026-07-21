package tools

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stackListServer struct {
	defangv1connect.UnimplementedFabricControllerHandler
}

func (stackListServer) ListStacks(context.Context, *connect.Request[defangv1.ListStacksRequest]) (*connect.Response[defangv1.ListStacksResponse], error) {
	return connect.NewResponse(&defangv1.ListStacksResponse{}), nil
}

func newStackTestFabric(t *testing.T) *client.GrpcClient {
	t.Helper()
	_, handler := defangv1connect.NewFabricControllerHandler(stackListServer{})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return client.NewGrpcClient(strings.TrimPrefix(server.URL, "http://"), "", types.TenantUnset)
}

func TestSetupProviderAndLoaderReusesLoaderWhenContextIsUnchanged(t *testing.T) {
	tests := []struct {
		name      string
		stack     stacks.Parameters
		supported bool
	}{
		{
			name:      "preselected stack",
			stack:     stacks.Parameters{Name: "production", Provider: client.ProviderAWS},
			supported: true,
		},
		{
			name:      "elicitations unsupported",
			stack:     stacks.Parameters{Provider: client.ProviderAuto},
			supported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			loader := &client.MockLoader{}
			ec := elicitations.NewController(&mockElicitationsClient{})
			ec.SetSupported(tt.supported)
			stack := tt.stack

			_, finalLoader, err := setupProviderAndLoader(t.Context(), loader, common.LoaderParams{}, &MockDeployCLI{}, ec, &client.GrpcClient{}, StackConfig{Stack: &stack})
			require.NoError(t, err)
			assert.Same(t, loader, finalLoader)
			assert.Equal(t, tt.stack, stack)
		})
	}
}

func TestSetupProviderAndLoaderPreservesWorkingDirectoryVariants(t *testing.T) {
	tests := []struct {
		name   string
		params func(root, app string) common.LoaderParams
	}{
		{
			name: "relative Compose path outside working directory",
			params: func(_, _ string) common.LoaderParams {
				return common.LoaderParams{WorkingDirectory: ".", ComposeFilePaths: []string{"app/compose.yaml"}}
			},
		},
		{
			name: "relative working directory with implicit Compose path",
			params: func(_, _ string) common.LoaderParams {
				return common.LoaderParams{WorkingDirectory: "app"}
			},
		},
		{
			name: "relative working directory with explicit Compose path",
			params: func(_, _ string) common.LoaderParams {
				return common.LoaderParams{WorkingDirectory: "app", ComposeFilePaths: []string{"compose.yaml"}}
			},
		},
		{
			name: "absolute working directory with relative Compose path",
			params: func(_, app string) common.LoaderParams {
				return common.LoaderParams{WorkingDirectory: app, ComposeFilePaths: []string{"compose.yaml"}}
			},
		},
		{
			name: "absolute Compose path",
			params: func(_, app string) common.LoaderParams {
				return common.LoaderParams{WorkingDirectory: ".", ComposeFilePaths: []string{filepath.Join(app, "compose.yaml")}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			app := filepath.Join(root, "app")
			require.NoError(t, os.MkdirAll(filepath.Join(app, ".defang"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(app, ".defang", "production"), []byte("DEFANG_PROVIDER=aws\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(app, "compose.yaml"), []byte(`name: agent-interpolation
services:
  web:
    image: alpine
    environment:
      PROVIDER: ${DEFANG_PROVIDER}
      STACK: ${DEFANG_STACK}
`), 0o644))
			t.Cleanup(func() {
				os.Unsetenv("DEFANG_PROVIDER")
				os.Unsetenv("DEFANG_STACK")
			})

			params := tt.params(root, app)
			params.ProjectName = "agent-interpolation"
			stack := &stacks.Parameters{Provider: client.ProviderAuto}
			loader, err := common.ConfigureAgentLoader(params, stack)
			require.NoError(t, err)
			ec := elicitations.NewController(&mockElicitationsClient{responses: map[string]string{
				"stack": "production [aws]",
			}})

			_, finalLoader, err := setupProviderAndLoader(t.Context(), loader, params, &MockDeployCLI{}, ec, newStackTestFabric(t), StackConfig{Stack: stack})
			require.NoError(t, err)
			project, err := finalLoader.LoadProject(t.Context())
			require.NoError(t, err)
			assert.Equal(t, app, project.WorkingDir)
			env := project.Services["web"].Environment
			require.NotNil(t, env["PROVIDER"])
			require.NotNil(t, env["STACK"])
			assert.Equal(t, "aws", *env["PROVIDER"])
			assert.Equal(t, "production", *env["STACK"])
		})
	}
}

type loaderWithoutResolver struct {
	client.Loader
}

type failingResolverLoader struct {
	client.Loader
}

func (failingResolverLoader) ResolveProjectWorkingDir(context.Context) (string, error) {
	return "", errors.New("resolve failed")
}

func TestSetupProviderAndLoaderRejectsUnresolvableLoaders(t *testing.T) {
	t.Chdir(t.TempDir())
	stack := &stacks.Parameters{Name: "production", Provider: client.ProviderAWS}
	ec := elicitations.NewController(&mockElicitationsClient{})

	_, _, err := setupProviderAndLoader(t.Context(), loaderWithoutResolver{Loader: &client.MockLoader{}}, common.LoaderParams{}, &MockDeployCLI{}, ec, &client.GrpcClient{}, StackConfig{Stack: stack})
	assert.EqualError(t, err, "loader tools.loaderWithoutResolver does not support resolving the project working directory")

	_, _, err = setupProviderAndLoader(t.Context(), failingResolverLoader{Loader: &client.MockLoader{}}, common.LoaderParams{}, &MockDeployCLI{}, ec, &client.GrpcClient{}, StackConfig{Stack: stack})
	assert.EqualError(t, err, "failed to get project working directory: resolve failed")
}
