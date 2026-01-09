package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	awsdriver "github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/bufbuild/connect-go"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
)

type MockSsmClient struct {
	awsdriver.SsmParametersAPI
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
		Region:            "us-test-2",
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

func TestMain(m *testing.M) {
	SetupCommands("0.0.0-test")
	os.Exit(m.Run())
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
			name:                "whoami - allowed",
			command:             []string{"whoami", "--provider=aws", "--dry-run"},
			expectCanIUseCalled: false,
		},
	}

	prevSts, prevSsm := awsdriver.NewStsFromConfig, awsdriver.NewSsmFromConfig
	t.Cleanup(func() {
		awsdriver.NewStsFromConfig = prevSts
		awsdriver.NewSsmFromConfig = prevSsm
	})
	awsdriver.NewStsFromConfig = func(aws.Config) awsdriver.StsClientAPI { return &awsdriver.MockStsClientAPI{} }
	awsdriver.NewSsmFromConfig = func(aws.Config) awsdriver.SsmParametersAPI { return &MockSsmClient{} }

	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
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

type mockStackManager struct {
	t                *testing.T
	expectedProvider client.ProviderID
	expectedRegion   string
	listResult       []stacks.StackListItem
	listError        error
	loadResults      map[string]*stacks.StackParameters
	loadResult       *stacks.StackParameters
	loadError        error
	createError      error
	createResult     *stacks.StackParameters
}

func NewMockStackManager(t *testing.T, expectedProvider client.ProviderID, expectedRegion string) *mockStackManager {
	return &mockStackManager{
		t:                t,
		expectedProvider: expectedProvider,
		expectedRegion:   expectedRegion,
		listResult:       []stacks.StackListItem{},
	}
}

func (m *mockStackManager) List(ctx context.Context) ([]stacks.StackListItem, error) {
	if m.listError != nil {
		return nil, m.listError
	}
	return m.listResult, nil
}

func (m *mockStackManager) Load(ctx context.Context, name string) (*stacks.StackParameters, error) {
	if m.loadError != nil {
		return nil, m.loadError
	}

	// Check for specific stack name first
	if m.loadResults != nil {
		if result, exists := m.loadResults[name]; exists {
			return result, nil
		}
	}

	// If we have an explicit loadResult, return it
	if m.loadResult != nil {
		return m.loadResult, nil
	}

	// If we have expected provider/region (from old NewMockStackManager usage), create default params
	if m.expectedProvider != "" && m.expectedRegion != "" {
		params := stacks.StackParameters{
			Name:     name,
			Provider: m.expectedProvider,
			Region:   m.expectedRegion,
			Mode:     modes.ModeAffordable,
		}
		stacks.LoadParameters(params, true)
		return &params, nil
	}

	return nil, fmt.Errorf("unable to find stack %q", name)
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

func (m *mockStackManager) LoadParameters(params stacks.StackParameters, overload bool) error {
	return stacks.LoadParameters(params, overload)
}

func TestNewProvider(t *testing.T) {
	mockClient := client.GrpcClient{}
	mockCtrl := &MockFabricControllerClient{
		canIUseResponse: defangv1.CanIUseResponse{},
	}
	mockClient.SetFabricClient(mockCtrl)
	oldRootCmd, oldClient := RootCmd, global.Client
	global.Stack = stacks.StackParameters{}
	t.Cleanup(func() {
		RootCmd = oldRootCmd
		global.Client = oldClient
		global.Stack = stacks.StackParameters{}
	})
	global.Client = &mockClient
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
		mockSM := NewMockStackManager(t, client.ProviderAWS, "us-test-2")

		p, err := newProvider(ctx, mockEC, mockSM, false)
		if err != nil {
			t.Fatalf("getProvider() failed: %v", err)
		}
		if _, ok := p.(*client.PlaygroundProvider); !ok {
			t.Errorf("Expected provider to be of type *cliClient.PlaygroundProvider, got %T", p)
		}
	})

	t.Run("Should set cd image from canIUse response", func(t *testing.T) {
		t.Chdir("../../../../src/testdata/sanity")

		global.Stack = stacks.StackParameters{
			Name: "beta",
		}
		// Set up RootCmd with required flags for getStack function
		RootCmd = &cobra.Command{Use: "defang"}
		RootCmd.PersistentFlags().StringVarP(&global.Stack.Name, "stack", "s", global.Stack.Name, "stack name")
		RootCmd.PersistentFlags().VarP(&global.Stack.Provider, "provider", "P", "provider")
		RootCmd.PersistentFlags().StringP("project-name", "p", "", "project name")
		RootCmd.PersistentFlags().StringArrayP("file", "f", []string{}, "compose file path(s)")

		// Parse the flags to initialize the flag system
		RootCmd.ParseFlags([]string{})

		prevSts := awsdriver.NewStsFromConfig
		awsdriver.NewStsFromConfig = func(cfg aws.Config) awsdriver.StsClientAPI { return &awsdriver.MockStsClientAPI{} }
		const cdImageTag = "site/registry/repo:tag@sha256:digest"
		mockCtrl.canIUseResponse.CdImage = cdImageTag
		t.Cleanup(func() {
			awsdriver.NewStsFromConfig = prevSts
			mockCtrl.canIUseResponse.CdImage = ""
			global.Stack = stacks.StackParameters{}
		})

		mockEC := &mockElicitationsController{}
		mockSM := NewMockStackManager(t, client.ProviderAWS, "us-test-2")
		p, err := newProvider(ctx, mockEC, mockSM, false)
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
		prevSts := awsdriver.NewStsFromConfig
		awsdriver.NewStsFromConfig = func(cfg aws.Config) awsdriver.StsClientAPI { return &awsdriver.MockStsClientAPI{} }
		const cdImageTag = "site/registry/repo:tag@sha256:digest"
		const overrideImageTag = "site/override/replaced:tag@sha256:otherdigest"
		t.Setenv("DEFANG_CD_IMAGE", overrideImageTag)
		mockCtrl.canIUseResponse.CdImage = cdImageTag
		global.Stack = stacks.StackParameters{
			Name: "beta",
		}
		t.Cleanup(func() {
			awsdriver.NewStsFromConfig = prevSts
			mockCtrl.canIUseResponse.CdImage = ""
			global.Stack = stacks.StackParameters{}
		})

		mockEC := &mockElicitationsController{}
		mockSM := NewMockStackManager(t, client.ProviderAWS, "us-test-2")
		p, err := newProvider(ctx, mockEC, mockSM, false)
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

func TestGetStack(t *testing.T) {
	ctx := context.Background()

	global.Stack = stacks.StackParameters{}
	// Save original state
	origGlobalNonInteractive := global.NonInteractive
	defer func() {
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
						Provider: client.ProviderAWS,
						Region:   "us-test-2",
					},
				}
				return ec, sm
			},
			stackFlag: "test-stack",
			expectedStack: &stacks.StackParameters{
				Name:     "test-stack",
				Provider: client.ProviderAWS,
				Region:   "us-test-2",
			},
			expectedWhence: "stack",
		},
		{
			name: "stack flag provided with unknown stack but no provider",
			setup: func(t *testing.T) (*mockElicitationsController, *mockStackManager) {
				ec := &mockElicitationsController{}
				sm := &mockStackManager{
					loadError: errors.New("stack not found"),
				}
				return ec, sm
			},
			stackFlag:     "nonexistent-stack",
			expectedError: "unable to find stack \"nonexistent-stack\": stack not found",
		},
		{
			name: "stack flag provided with unknown stack but valid provider",
			setup: func(t *testing.T) (*mockElicitationsController, *mockStackManager) {
				ec := &mockElicitationsController{}
				sm := &mockStackManager{
					loadError: errors.New("stack not found"),
				}
				return ec, sm
			},
			stackFlag:     "nonexistent-stack",
			providerFlag:  "gcp",
			expectedError: "unable to find stack \"nonexistent-stack\": stack not found",
		},
		{
			name: "stack flag with auto provider should error",
			setup: func(t *testing.T) (*mockElicitationsController, *mockStackManager) {
				ec := &mockElicitationsController{}
				sm := &mockStackManager{
					loadResult: &stacks.StackParameters{
						Name:     "auto-stack",
						Provider: client.ProviderAuto,
						Region:   "us-test-2",
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
						Provider: client.ProviderAWS,
					},
				}
				return ec, sm
			},
			providerFlag:  "aws",
			expectWarning: true,
			expectedStack: &stacks.StackParameters{
				Name:     "existing-stack",
				Provider: client.ProviderAWS,
			},
			expectedWhence: "only stack",
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
						Provider: client.ProviderAWS,
					},
				}
				return ec, sm
			},
			envProvider:   "gcp",
			expectWarning: true,
			expectedStack: &stacks.StackParameters{
				Name:     "existing-stack",
				Provider: client.ProviderAWS,
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
				Provider: client.ProviderDefang,
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
						{Name: "only-stack", Provider: "aws", Mode: "affordable"},
					},
					loadResult: &stacks.StackParameters{
						Name:     "only-stack",
						Provider: client.ProviderAWS,
						Mode:     modes.ModeAffordable,
					},
				}
				return ec, sm
			},
			expectedStack: &stacks.StackParameters{
				Name:     "only-stack",
				Provider: client.ProviderAWS,
				Mode:     modes.ModeAffordable,
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
						Provider: client.ProviderAWS,
					},
				}
				return ec, sm
			},
			expectedStack: &stacks.StackParameters{
				Name:     "stack1",
				Provider: client.ProviderAWS,
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
						{Name: "stack2", Provider: "aws"},
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

			provider := client.ProviderAuto
			provider.Set(tc.providerFlag)
			global.Stack = stacks.StackParameters{
				Name:     tc.stackFlag,
				Provider: provider,
			}

			global.NonInteractive = tc.nonInteractive

			// Capture output to check for warnings
			var output bytes.Buffer

			// Call the function under test
			stack, whence, err := getStack(ctx, ec, sm, true)

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

			_ = output // Suppress unused variable warning for now
		})
	}
}

func TestConfigSetMultiple(t *testing.T) {
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

	prevSts, prevSsm := awsdriver.NewStsFromConfig, awsdriver.NewSsmFromConfig
	t.Cleanup(func() {
		awsdriver.NewStsFromConfig = prevSts
		awsdriver.NewSsmFromConfig = prevSsm
	})
	awsdriver.NewStsFromConfig = func(aws.Config) awsdriver.StsClientAPI { return &awsdriver.MockStsClientAPI{} }
	awsdriver.NewSsmFromConfig = func(aws.Config) awsdriver.SsmParametersAPI { return &MockSsmClient{} }

	testCases := []struct {
		name        string
		args        []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "multiple configs with one missing = should error",
			args:        []string{"config", "set", "KEY1=value1", "KEY2", "--provider=aws", "--project-name=app"},
			expectError: true,
			errorMsg:    "when setting multiple configs, all must be in KEY=VALUE format",
		},
		{
			name:        "multiple configs with --env should error",
			args:        []string{"config", "set", "KEY1=value1", "KEY2=value2", "-e", "--provider=aws", "--project-name=app"},
			expectError: true,
			errorMsg:    "--env is only allowed when setting a single config",
		},
		{
			name:        "multiple configs with --random should error",
			args:        []string{"config", "set", "KEY1=value1", "KEY2=value2", "--random", "--provider=aws", "--project-name=app"},
			expectError: true,
			errorMsg:    "--random is only allowed when setting a single config",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := testCommand(tc.args, server.URL)

			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tc.errorMsg != "" && !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("expected error message to contain %q, got %q", tc.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
