package command

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/auth"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	pkg "github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/ptr"
	"github.com/bufbuild/connect-go"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
)

type MockSsmClient struct {
	pkg.SsmParametersAPI
}

func (m *MockSsmClient) PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	return &ssm.PutParameterOutput{}, nil
}

func (m *MockSsmClient) DeleteParameters(ctx context.Context, params *ssm.DeleteParametersInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParametersOutput, error) {
	return &ssm.DeleteParametersOutput{
		DeletedParameters: []string{"var"},
	}, nil
}

type mockFabricService struct {
	defangv1connect.UnimplementedFabricControllerHandler
	canIUseIsCalled bool
}

func (m *mockFabricService) CanIUse(ctx context.Context, canUseReq *connect.Request[defangv1.CanIUseRequest]) (*connect.Response[defangv1.CanIUseResponse], error) {
	m.canIUseIsCalled = true
	return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("no access to use aws provider"))
}

func (m *mockFabricService) GetVersion(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[defangv1.Version], error) {
	return connect.NewResponse(&defangv1.Version{
		Fabric: "0.0.0-test",
		CliMin: "0.0.0-test",
	}), nil
}

func (m *mockFabricService) CheckToS(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (m *mockFabricService) WhoAmI(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[defangv1.WhoAmIResponse], error) {
	return connect.NewResponse(&defangv1.WhoAmIResponse{
		Tenant:            "default",
		ProviderAccountId: "default",
		Region:            "us-west-2",
		Tier:              defangv1.SubscriptionTier_HOBBY,
	}), nil
}

func (m *mockFabricService) GetSelectedProvider(context.Context, *connect.Request[defangv1.GetSelectedProviderRequest]) (*connect.Response[defangv1.GetSelectedProviderResponse], error) {
	return connect.NewResponse(&defangv1.GetSelectedProviderResponse{
		Provider: defangv1.Provider_AWS,
	}), nil
}

func (m *mockFabricService) SetSelectedProvider(context.Context, *connect.Request[defangv1.SetSelectedProviderRequest]) (*connect.Response[emptypb.Empty], error) {
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (m *mockFabricService) ListDeployments(context.Context, *connect.Request[defangv1.ListDeploymentsRequest]) (*connect.Response[defangv1.ListDeploymentsResponse], error) {
	return connect.NewResponse(&defangv1.ListDeploymentsResponse{
		Deployments: []*defangv1.Deployment{
			{
				Stack:    "beta",
				Provider: defangv1.Provider_AWS,
				Mode:     defangv1.DeploymentMode_DEVELOPMENT,
			},
		},
	}), nil
}

func init() {
	SetupCommands(context.Background(), "0.0.0-test")
}

type mockStsProviderAPI struct{}

func (s *mockStsProviderAPI) GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	callIdOutput := sts.GetCallerIdentityOutput{}
	callIdOutput.Account = ptr.String("123456789012")
	callIdOutput.Arn = ptr.String("arn:aws:iam::123456789012:user/test")

	return &callIdOutput, nil
}

func (s *mockStsProviderAPI) AssumeRole(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	aro := sts.AssumeRoleOutput{}
	return &aro, nil
}

func testCommand(args []string, cluster string) error {
	if cluster != "" {
		args = append(args, "--cluster", strings.TrimPrefix(cluster, "http://"))
	}
	RootCmd.SetArgs(args)
	return RootCmd.ExecuteContext(context.Background())
}

func TestVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live tests in short mode.")
	}

	t.Run("live", func(t *testing.T) {
		err := testCommand([]string{"version"}, "")
		if err != nil {
			t.Fatalf("Version() failed: %v", err)
		}
	})

	t.Run("mock", func(t *testing.T) {
		mockService := &mockFabricService{}
		_, handler := defangv1connect.NewFabricControllerHandler(mockService)

		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)

		err := testCommand([]string{"version"}, server.URL)
		if err != nil {
			t.Fatalf("Version() failed: %v", err)
		}
	})
}

func TestCommandGates(t *testing.T) {
	mockService := &mockFabricService{}
	_, handler := defangv1connect.NewFabricControllerHandler(mockService)
	t.Chdir("../../../../src/testdata/sanity")

	userinfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/userinfo" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"allTenants":[{"id":"default","name":"Default Workspace"}],
			"userinfo":{"email":"cli@example.com","name":"CLI Tester"}
		}`))
	}))
	t.Cleanup(userinfoServer.Close)

	openAuthClient := auth.OpenAuthClient
	t.Cleanup(func() {
		auth.OpenAuthClient = openAuthClient
	})
	auth.OpenAuthClient = auth.NewClient("testclient", userinfoServer.URL)
	t.Setenv("DEFANG_ACCESS_TOKEN", "token-123")

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	testData := []struct {
		name                string
		command             []string
		expectCanIUseCalled bool
	}{
		{
			name:                "compose up - aws - no access",
			command:             []string{"compose", "up", "--provider=aws", "--dry-run"},
			expectCanIUseCalled: true,
		},
		{
			name:                "compose down - aws - no access",
			command:             []string{"compose", "down", "--provider=aws", "--project-name=myproj", "--dry-run"},
			expectCanIUseCalled: true,
		},
		{
			name:                "config set - aws - allowed",
			command:             []string{"config", "set", "var", "--project-name=app", "--provider=aws", "--dry-run"},
			expectCanIUseCalled: false,
		},
		{
			name:                "delete service - aws - no access",
			command:             []string{"delete", "abc", "--provider=aws", "--dry-run"},
			expectCanIUseCalled: true,
		},
		{
			name:                "whoami - allowed",
			command:             []string{"whoami", "--provider=aws", "--dry-run"},
			expectCanIUseCalled: false,
		},
	}

	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			aws.StsClient = &mockStsProviderAPI{}
			pkg.SsmClientOverride = &MockSsmClient{}
			mockService.canIUseIsCalled = false

			err := testCommand(tt.command, server.URL)

			if tt.expectCanIUseCalled != mockService.canIUseIsCalled {
				t.Errorf("unexpected canIUse: expected usage: %t", tt.expectCanIUseCalled)
			}

			if err != nil {
				if tt.expectCanIUseCalled && err.Error() != "resource_exhausted: no access to use aws provider" {
					t.Errorf("expected \"no access\" error - got: %v", err.Error())
				}
			}
		})
	}
}

type MockFabricControllerClient struct {
	defangv1connect.FabricControllerClient
	canIUseResponse defangv1.CanIUseResponse
	savedProvider   map[string]defangv1.Provider
}

func (m *MockFabricControllerClient) CanIUse(context.Context, *connect.Request[defangv1.CanIUseRequest]) (*connect.Response[defangv1.CanIUseResponse], error) {
	return connect.NewResponse(&m.canIUseResponse), nil
}

func (m *MockFabricControllerClient) GetServices(context.Context, *connect.Request[defangv1.GetServicesRequest]) (*connect.Response[defangv1.GetServicesResponse], error) {
	return connect.NewResponse(&defangv1.GetServicesResponse{}), nil
}

func (m *MockFabricControllerClient) GetSelectedProvider(ctx context.Context, req *connect.Request[defangv1.GetSelectedProviderRequest]) (*connect.Response[defangv1.GetSelectedProviderResponse], error) {
	return connect.NewResponse(&defangv1.GetSelectedProviderResponse{
		Provider: m.savedProvider[req.Msg.Project],
	}), nil
}

func (m *MockFabricControllerClient) SetSelectedProvider(ctx context.Context, req *connect.Request[defangv1.SetSelectedProviderRequest]) (*connect.Response[emptypb.Empty], error) {
	if m.savedProvider == nil {
		m.savedProvider = make(map[string]defangv1.Provider)
	}
	m.savedProvider[req.Msg.Project] = req.Msg.Provider
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (m *MockFabricControllerClient) ListDeployments(ctx context.Context, req *connect.Request[defangv1.ListDeploymentsRequest]) (*connect.Response[defangv1.ListDeploymentsResponse], error) {
	return connect.NewResponse(&defangv1.ListDeploymentsResponse{
		Deployments: []*defangv1.Deployment{},
	}), nil
}

type FakeStdin struct {
	*bytes.Reader
}

func (f *FakeStdin) Fd() uintptr {
	return os.Stdin.Fd()
}

type FakeStdout struct {
	*bytes.Buffer
}

func (f *FakeStdout) Fd() uintptr {
	return os.Stdout.Fd()
}

type mockStacksManager struct {
	t *testing.T
	stacks.Manager
	expectedProvider cliClient.ProviderID
	expectedRegion   string
}

func NewMockStacksManager(t *testing.T, expectedProvider cliClient.ProviderID, expectedRegion string) *mockStacksManager {
	return &mockStacksManager{
		t:                t,
		expectedProvider: expectedProvider,
		expectedRegion:   expectedRegion,
	}
}

func (m *mockStacksManager) List(ctx context.Context) ([]stacks.StackListItem, error) {
	return []stacks.StackListItem{}, nil
}

func (m *mockStacksManager) Load(name string) (*stacks.StackParameters, error) {
	params := stacks.StackParameters{
		Name:     name,
		Provider: m.expectedProvider,
		Region:   m.expectedRegion,
	}

	m.LoadParameters(params.ToMap(), true)

	return &params, nil
}

func (m *mockStacksManager) LoadParameters(params map[string]string, overload bool) error {
	return stacks.LoadParameters(params, overload)
}

func (m *mockStacksManager) Create(params stacks.StackParameters) (string, error) {
	return params.Name, nil
}

func TestNewProvider(t *testing.T) {
	mockClient := cliClient.GrpcClient{}
	mockCtrl := &MockFabricControllerClient{
		canIUseResponse: defangv1.CanIUseResponse{},
	}
	mockClient.SetClient(mockCtrl)
	global.Client = &mockClient
	oldRootCmd := RootCmd
	t.Cleanup(func() {
		RootCmd = oldRootCmd
		global.Stack = stacks.StackParameters{}
	})
	FakeRootWithProviderParam := func(provider string) *cobra.Command {
		cmd := &cobra.Command{}
		cmd.PersistentFlags().VarP(&global.Stack.Provider, "provider", "P", "fake provider flag")
		if provider != "" {
			cmd.ParseFlags([]string{"--provider", provider})
		}
		return cmd
	}

	ctx := t.Context()

	t.Run("Nil loader auto provider non-interactive should load playground provider", func(t *testing.T) {
		global.Stack.Provider = "auto"
		os.Unsetenv("DEFANG_PROVIDER")
		RootCmd = FakeRootWithProviderParam("")

		// Create a mock stacks manager that returns empty stack list
		mockEC := &mockElicitationsController{}
		mockSM := NewMockStacksManager(t, cliClient.ProviderAWS, "us-west-2")

		p, err := newProvider(ctx, mockEC, mockSM)
		if err != nil {
			t.Fatalf("getProvider() failed: %v", err)
		}
		if _, ok := p.(*cliClient.PlaygroundProvider); !ok {
			t.Errorf("Expected provider to be of type *cliClient.PlaygroundProvider, got %T", p)
		}
	})

	t.Run("Should set cd image from canIUse response", func(t *testing.T) {
		t.Chdir("../../../../src/testdata/sanity")
		t.Setenv("DEFANG_STACK", "beta")

		// Set up RootCmd with required flags for getStack function
		RootCmd = &cobra.Command{Use: "defang"}
		RootCmd.PersistentFlags().StringVarP(&global.Stack.Name, "stack", "s", global.Stack.Name, "stack name")
		RootCmd.PersistentFlags().VarP(&global.Stack.Provider, "provider", "P", "provider")
		RootCmd.PersistentFlags().StringP("project-name", "p", "", "project name")
		RootCmd.PersistentFlags().StringArrayP("file", "f", []string{}, "compose file path(s)")

		// Parse the flags to initialize the flag system
		RootCmd.ParseFlags([]string{})

		sts := aws.StsClient
		aws.StsClient = &mockStsProviderAPI{}
		const cdImageTag = "site/registry/repo:tag@sha256:digest"
		mockCtrl.canIUseResponse.CdImage = cdImageTag
		t.Cleanup(func() {
			aws.StsClient = sts
			mockCtrl.canIUseResponse.CdImage = ""
			global.Stack = stacks.StackParameters{}
		})

		mockEC := &mockElicitationsController{}
		mockSM := NewMockStacksManager(t, cliClient.ProviderAWS, "us-west-2")
		p, err := newProvider(ctx, mockEC, mockSM)
		if err != nil {
			t.Errorf("getProvider() failed: %v", err)
		}

		err = canIUseProvider(ctx, p, "project", 0)
		if err != nil {
			t.Errorf("CanIUseProvider() failed: %v", err)
		}

		if awsProvider, ok := p.(*aws.ByocAws); !ok {
			t.Errorf("Expected provider to be of type *aws.ByocAws, got %T", p)
		} else {
			if awsProvider.CDImage != cdImageTag {
				t.Errorf("Expected cd image tag to be %s, got: %s", cdImageTag, awsProvider.CDImage)
			}
		}
	})

	t.Run("Can override cd image from environment variable", func(t *testing.T) {
		t.Chdir("../../../../src/testdata/sanity")
		t.Setenv("DEFANG_STACK", "beta")
		sts := aws.StsClient
		aws.StsClient = &mockStsProviderAPI{}
		const cdImageTag = "site/registry/repo:tag@sha256:digest"
		const overrideImageTag = "site/override/replaced:tag@sha256:otherdigest"
		t.Setenv("DEFANG_CD_IMAGE", overrideImageTag)
		mockCtrl.canIUseResponse.CdImage = cdImageTag
		t.Cleanup(func() {
			aws.StsClient = sts
			mockCtrl.canIUseResponse.CdImage = ""
			global.Stack = stacks.StackParameters{}
		})

		mockEC := &mockElicitationsController{}
		mockSM := NewMockStacksManager(t, cliClient.ProviderAWS, "us-west-2")
		p, err := newProvider(ctx, mockEC, mockSM)
		if err != nil {
			t.Errorf("getProvider() failed: %v", err)
		}

		err = canIUseProvider(ctx, p, "project", 0)
		if err != nil {
			t.Errorf("CanIUseProvider() failed: %v", err)
		}

		if awsProvider, ok := p.(*aws.ByocAws); !ok {
			t.Errorf("Expected provider to be of type *aws.ByocAws, got %T", p)
		} else {
			if awsProvider.CDImage != overrideImageTag {
				t.Errorf("Expected cd image tag to be %s, got: %s", cdImageTag, awsProvider.CDImage)
			}
		}
	})
}

type mockElicitationsController struct {
	isSupported bool
	enumChoice  string
}

func (m *mockElicitationsController) RequestString(ctx context.Context, message, field string) (string, error) {
	return "", nil
}

func (m *mockElicitationsController) RequestStringWithDefault(ctx context.Context, message, field, defaultValue string) (string, error) {
	return defaultValue, nil
}

func (m *mockElicitationsController) RequestEnum(ctx context.Context, message, field string, options []string) (string, error) {
	if m.enumChoice != "" {
		return m.enumChoice, nil
	}
	if len(options) > 0 {
		return options[0], nil
	}
	return "", nil
}

func (m *mockElicitationsController) SetSupported(supported bool) {
	m.isSupported = supported
}

func (m *mockElicitationsController) IsSupported() bool {
	return m.isSupported
}

type mockStackManager struct {
	listResult   []stacks.StackListItem
	listError    error
	loadResults  map[string]*stacks.StackParameters
	loadResult   *stacks.StackParameters
	loadError    error
	createError  error
	createResult *stacks.StackParameters
}

func (m *mockStackManager) List(ctx context.Context) ([]stacks.StackListItem, error) {
	if m.listError != nil {
		return nil, m.listError
	}
	return m.listResult, nil
}

func (m *mockStackManager) Load(name string) (*stacks.StackParameters, error) {
	if m.loadError != nil {
		return nil, m.loadError
	}

	// Check for specific stack name first
	if m.loadResults != nil {
		if result, exists := m.loadResults[name]; exists {
			return result, nil
		}
	}

	// Fall back to default
	return m.loadResult, nil
}

func (m *mockStackManager) Create(params stacks.StackParameters) (string, error) {
	if m.createError != nil {
		return "", m.createError
	}
	if m.createResult != nil {
		m.loadResult = m.createResult
	}
	return params.Name, nil
}

func (m *mockStackManager) LoadParameters(params map[string]string, overload bool) error {
	return stacks.LoadParameters(params, overload)
}

func TestGetStack(t *testing.T) {
	ctx := context.Background()

	// Save original state
	origRootCmd := RootCmd
	origGlobalNonInteractive := global.NonInteractive
	defer func() {
		RootCmd = origRootCmd
		global.NonInteractive = origGlobalNonInteractive
		global.Stack = stacks.StackParameters{}
	}()

	testCases := []struct {
		name           string
		setup          func(t *testing.T) (*mockElicitationsController, *mockStackManager)
		stackFlag      string
		providerFlag   string
		envProvider    string
		nonInteractive bool
		expectedStack  *stacks.StackParameters
		expectedWhence string
		expectedError  string
		expectWarning  bool
	}{
		{
			name: "stack flag provided with valid stack",
			setup: func(t *testing.T) (*mockElicitationsController, *mockStackManager) {
				ec := &mockElicitationsController{}
				sm := &mockStackManager{
					loadResult: &stacks.StackParameters{
						Name:     "test-stack",
						Provider: cliClient.ProviderAWS,
						Region:   "us-west-2",
					},
				}
				return ec, sm
			},
			stackFlag: "test-stack",
			expectedStack: &stacks.StackParameters{
				Name:     "test-stack",
				Provider: cliClient.ProviderAWS,
				Region:   "us-west-2",
			},
			expectedWhence: "stack file",
		},
		{
			name: "stack flag provided with invalid stack",
			setup: func(t *testing.T) (*mockElicitationsController, *mockStackManager) {
				ec := &mockElicitationsController{}
				sm := &mockStackManager{
					loadError: errors.New("stack not found"),
				}
				return ec, sm
			},
			stackFlag:     "nonexistent-stack",
			expectedError: "unable to load stack \"nonexistent-stack\": stack not found",
		},
		{
			name: "stack flag with auto provider should error",
			setup: func(t *testing.T) (*mockElicitationsController, *mockStackManager) {
				ec := &mockElicitationsController{}
				sm := &mockStackManager{
					loadResult: &stacks.StackParameters{
						Name:     "auto-stack",
						Provider: cliClient.ProviderAuto,
						Region:   "us-west-2",
					},
				}
				return ec, sm
			},
			stackFlag:     "auto-stack",
			expectedError: "stack \"auto-stack\" has an invalid provider \"auto\"",
		},
		{
			name: "provider flag provided with warning and existing stacks",
			setup: func(t *testing.T) (*mockElicitationsController, *mockStackManager) {
				ec := &mockElicitationsController{
					isSupported: true,
					enumChoice:  "existing-stack",
				}
				sm := &mockStackManager{
					listResult: []stacks.StackListItem{
						{Name: "existing-stack", Provider: "aws"},
					},
					loadResult: &stacks.StackParameters{
						Name:     "existing-stack",
						Provider: cliClient.ProviderAWS,
					},
				}
				return ec, sm
			},
			providerFlag:  "aws",
			expectWarning: true,
			expectedStack: &stacks.StackParameters{
				Name:     "existing-stack",
				Provider: cliClient.ProviderAWS,
			},
			expectedWhence: "interactive selection",
		},
		{
			name: "env provider with warning and existing stacks",
			setup: func(t *testing.T) (*mockElicitationsController, *mockStackManager) {
				ec := &mockElicitationsController{
					isSupported: true,
					enumChoice:  "existing-stack",
				}
				sm := &mockStackManager{
					listResult: []stacks.StackListItem{
						{Name: "existing-stack", Provider: "aws"}, // Different provider to avoid "only stack" path
						{Name: "other-stack", Provider: "gcp"},
					},
					loadResult: &stacks.StackParameters{
						Name:     "existing-stack",
						Provider: cliClient.ProviderAWS,
					},
				}
				return ec, sm
			},
			envProvider:   "gcp",
			expectWarning: true,
			expectedStack: &stacks.StackParameters{
				Name:     "existing-stack",
				Provider: cliClient.ProviderAWS,
			},
			expectedWhence: "interactive selection",
		},
		{
			name: "non-interactive with auto provider returns default",
			setup: func(t *testing.T) (*mockElicitationsController, *mockStackManager) {
				ec := &mockElicitationsController{}
				sm := &mockStackManager{
					listResult: []stacks.StackListItem{},
				}
				return ec, sm
			},
			nonInteractive: true,
			expectedStack: &stacks.StackParameters{
				Name:     "beta",
				Provider: cliClient.ProviderDefang,
				Mode:     modes.ModeUnspecified,
			},
			expectedWhence: "non-interactive default",
		},
		{
			name: "single stack matches provider",
			setup: func(t *testing.T) (*mockElicitationsController, *mockStackManager) {
				ec := &mockElicitationsController{}
				sm := &mockStackManager{
					listResult: []stacks.StackListItem{
						{Name: "only-stack", Provider: "auto"},
					},
				}
				return ec, sm
			},
			expectedStack: &stacks.StackParameters{
				Name:     "only-stack",
				Provider: cliClient.ProviderAuto,
			},
			expectedWhence: "only stack",
		},
		{
			name: "interactive selection succeeds",
			setup: func(t *testing.T) (*mockElicitationsController, *mockStackManager) {
				ec := &mockElicitationsController{
					isSupported: true,
					enumChoice:  "stack1",
				}
				sm := &mockStackManager{
					listResult: []stacks.StackListItem{
						{Name: "stack1", Provider: "aws"},
						{Name: "stack2", Provider: "gcp"},
					},
					loadResult: &stacks.StackParameters{
						Name:     "stack1",
						Provider: cliClient.ProviderAWS,
					},
				}
				return ec, sm
			},
			expectedStack: &stacks.StackParameters{
				Name:     "stack1",
				Provider: cliClient.ProviderAWS,
			},
			expectedWhence: "interactive selection",
		},
		{
			name: "sm.List error should propagate",
			setup: func(t *testing.T) (*mockElicitationsController, *mockStackManager) {
				ec := &mockElicitationsController{}
				sm := &mockStackManager{
					listError: errors.New("failed to list stacks"),
				}
				return ec, sm
			},
			expectedError: "unable to list stacks: failed to list stacks",
		},
		{
			name: "stackSelector.SelectStack error should propagate",
			setup: func(t *testing.T) (*mockElicitationsController, *mockStackManager) {
				ec := &mockElicitationsController{isSupported: false} // Will cause SelectStack to fail
				sm := &mockStackManager{
					listResult: []stacks.StackListItem{
						{Name: "stack1", Provider: "aws"},
					},
				}
				return ec, sm
			},
			expectedError: "failed to select stack:",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mocks
			ec, sm := tc.setup(t)

			// Create a new root command for this test
			testRootCmd := &cobra.Command{Use: "defang"}
			testRootCmd.PersistentFlags().String("stack", "", "stack name")
			testRootCmd.PersistentFlags().VarP(&global.Stack.Provider, "provider", "P", "provider")

			// Set flags if provided
			var args []string
			if tc.stackFlag != "" {
				args = append(args, "--stack", tc.stackFlag)
			}
			if tc.providerFlag != "" {
				args = append(args, "--provider", tc.providerFlag)
			}

			if len(args) > 0 {
				testRootCmd.ParseFlags(args)
			}

			// Set environment variable if provided
			if tc.envProvider != "" {
				t.Setenv("DEFANG_PROVIDER", tc.envProvider)
			} else {
				os.Unsetenv("DEFANG_PROVIDER")
			}

			// Set global state
			RootCmd = testRootCmd
			global.NonInteractive = tc.nonInteractive

			// Reset global stack state
			global.Stack.Provider = cliClient.ProviderAuto

			// Capture output to check for warnings
			var output bytes.Buffer

			// Call the function under test
			stack, whence, err := getStack(ctx, ec, sm)

			// Check error expectations
			if tc.expectedError != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tc.expectedError)
				}
				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Fatalf("expected error to contain %q, got %q", tc.expectedError, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check stack expectations
			if tc.expectedStack != nil {
				if stack == nil {
					t.Fatal("expected stack to be non-nil")
				}
				if stack.Name != tc.expectedStack.Name {
					t.Errorf("expected stack name %q, got %q", tc.expectedStack.Name, stack.Name)
				}
				if stack.Provider != tc.expectedStack.Provider {
					t.Errorf("expected stack provider %q, got %q", tc.expectedStack.Provider, stack.Provider)
				}
				if tc.expectedStack.Region != "" && stack.Region != tc.expectedStack.Region {
					t.Errorf("expected stack region %q, got %q", tc.expectedStack.Region, stack.Region)
				}
			}

			// Check whence expectations
			if tc.expectedWhence != "" && whence != tc.expectedWhence {
				t.Errorf("expected whence %q, got %q", tc.expectedWhence, whence)
			}

			// Check warning expectations
			if tc.expectWarning {
				// Since we can't easily capture term.Warn output in tests, we just verify
				// that the code path that would produce warnings was taken
				if tc.providerFlag != "" && !testRootCmd.PersistentFlags().Changed("provider") {
					t.Error("expected provider flag to be marked as changed for warning path")
				}
			}

			_ = output // Suppress unused variable warning for now
		})
	}
}
