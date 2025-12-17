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
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/gcp"
	pkg "github.com/DefangLabs/defang/src/pkg/clouds/aws"
	gcpdriver "github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/ptr"
	"github.com/bufbuild/connect-go"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2/google"
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
		Deployments: []*defangv1.Deployment{},
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

	t.Setenv("AWS_REGION", "us-west-2")

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

type mockElicitationsClient struct {
	responses map[string]string
}

func (m *mockElicitationsClient) Request(ctx context.Context, req elicitations.Request) (elicitations.Response, error) {
	properties, ok := req.Schema["properties"].(map[string]any)
	if !ok || len(properties) == 0 {
		panic("invalid schema properties")
	}
	fields := make([]string, 0)
	for field := range properties {
		fields = append(fields, field)
	}

	if len(fields) > 1 {
		panic("mockElicitationsClient only supports single-field requests")
	}

	return elicitations.Response{
		Action: "accept",
		Content: map[string]any{
			fields[0]: m.responses[fields[0]],
		},
	}, nil
}

type mockStacksManager struct {
	stacks.Manager
	expectedProvider cliClient.ProviderID
}

func (m *mockStacksManager) List(ctx context.Context) ([]stacks.StackListItem, error) {
	return []stacks.StackListItem{}, nil
}

func (m *mockStacksManager) Load(name string) (*stacks.StackParameters, error) {
	return &stacks.StackParameters{
		Name:     name,
		Provider: m.expectedProvider,
		Region:   "us-west-2",
	}, nil
}

func (m *mockStacksManager) Create(params stacks.StackParameters) (string, error) {
	return params.Name, nil
}

func TestGetProvider(t *testing.T) {
	mockClient := cliClient.GrpcClient{}
	mockCtrl := &MockFabricControllerClient{
		canIUseResponse: defangv1.CanIUseResponse{},
	}
	mockClient.SetClient(mockCtrl)
	global.Client = &mockClient
	oldRootCmd := RootCmd
	t.Cleanup(func() {
		RootCmd = oldRootCmd
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
		mockSM := &mockStacksManager{}

		p, err := newProvider(ctx, nil, mockSM, "empty")
		if err != nil {
			t.Fatalf("getProvider() failed: %v", err)
		}
		if _, ok := p.(*cliClient.PlaygroundProvider); !ok {
			t.Errorf("Expected provider to be of type *cliClient.PlaygroundProvider, got %T", p)
		}
	})

	t.Run("Auto provider should get provider from client", func(t *testing.T) {
		global.Stack.Provider = "auto"
		os.Unsetenv("DEFANG_PROVIDER")
		t.Setenv("AWS_REGION", "us-west-2")
		RootCmd = FakeRootWithProviderParam("")

		mockCtrl.savedProvider = map[string]defangv1.Provider{"empty": defangv1.Provider_AWS}

		ni := global.NonInteractive
		sts := aws.StsClient
		aws.StsClient = &mockStsProviderAPI{}
		global.NonInteractive = false
		t.Cleanup(func() {
			global.NonInteractive = ni
			aws.StsClient = sts
			mockCtrl.savedProvider = nil
		})

		mockSM := &mockStacksManager{}
		p, err := newProvider(ctx, nil, mockSM, "empty")
		if err != nil {
			t.Fatalf("getProvider() failed: %v", err)
		}
		if _, ok := p.(*aws.ByocAws); !ok {
			t.Errorf("Expected provider to be of type *aws.ByocAws, got %T", p)
		}
	})

	t.Run("Auto provider from param with saved provider should go interactive and save", func(t *testing.T) {
		global.Stack.Provider = "auto"
		os.Unsetenv("DEFANG_PROVIDER")
		t.Setenv("AWS_REGION", "us-west-2")
		mockCtrl.savedProvider = map[string]defangv1.Provider{"someotherproj": defangv1.Provider_AWS}
		RootCmd = FakeRootWithProviderParam("")

		ni := global.NonInteractive
		sts := aws.StsClient
		aws.StsClient = &mockStsProviderAPI{}
		global.NonInteractive = false
		t.Cleanup(func() {
			global.NonInteractive = ni
			aws.StsClient = sts
			mockCtrl.savedProvider = nil
		})

		mockElicitationsClient := &mockElicitationsClient{
			responses: map[string]string{
				"provider":    "AWS",
				"region":      "us-west-2",
				"stack_name":  "teststack",
				"aws_profile": "default",
			},
		}
		ec := elicitations.NewController(mockElicitationsClient)
		sm := &mockStacksManager{expectedProvider: cliClient.ProviderAWS}
		p, err := newProvider(ctx, ec, sm, "empty")
		if err != nil {
			t.Fatalf("getProvider() failed: %v", err)
		}
		if _, ok := p.(*aws.ByocAws); !ok {
			t.Errorf("Expected provider to be of type *aws.ByocAws, got %T", p)
		}
		if mockCtrl.savedProvider["empty"] != defangv1.Provider_AWS {
			t.Errorf("Expected provider to be saved as AWS, got %v", mockCtrl.savedProvider["empty"])
		}
	})

	t.Run("Interactive provider prompt infer default provider from environment variable", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skip digitalocean test")
		}
		global.Stack.Provider = "auto"
		os.Unsetenv("DEFANG_PROVIDER")
		os.Unsetenv("AWS_PROFILE")
		t.Setenv("AWS_REGION", "us-west-2")
		t.Setenv("DIGITALOCEAN_TOKEN", "test-token")
		mockCtrl.savedProvider = map[string]defangv1.Provider{"someotherproj": defangv1.Provider_AWS}
		RootCmd = FakeRootWithProviderParam("")

		ni := global.NonInteractive
		sts := aws.StsClient
		aws.StsClient = &mockStsProviderAPI{}
		global.NonInteractive = false
		t.Cleanup(func() {
			global.NonInteractive = ni
			aws.StsClient = sts
			mockCtrl.savedProvider = nil
		})

		mockElicitationsClient := &mockElicitationsClient{
			responses: map[string]string{
				"provider": "DigitalOcean",
				"region":   "nyc3",
			},
		}
		ec := elicitations.NewController(mockElicitationsClient)
		sm := &mockStacksManager{expectedProvider: cliClient.ProviderDO}
		_, err := newProvider(ctx, ec, sm, "empty")
		if err != nil && !strings.HasPrefix(err.Error(), "GET https://api.digitalocean.com/v2/account: 401") {
			t.Fatalf("getProvider() failed: %v", err)
		}
		if mockCtrl.savedProvider["empty"] != defangv1.Provider_DIGITALOCEAN {
			t.Errorf("Expected provider to be saved as DIGITALOCEAN, got %v", mockCtrl.savedProvider["empty"])
		}
	})

	t.Run("Auto provider from param with saved provider should go interactive and save", func(t *testing.T) {
		os.Unsetenv("GCP_PROJECT_ID") // To trigger error
		os.Unsetenv("DEFANG_PROVIDER")
		global.Stack.Provider = "auto"
		mockCtrl.savedProvider = map[string]defangv1.Provider{"empty": defangv1.Provider_AWS}
		RootCmd = FakeRootWithProviderParam("auto")

		ni := global.NonInteractive
		sts := aws.StsClient
		aws.StsClient = &mockStsProviderAPI{}
		global.NonInteractive = false
		t.Cleanup(func() {
			global.NonInteractive = ni
			aws.StsClient = sts
			mockCtrl.savedProvider = nil
		})

		mockElicitationsClient := &mockElicitationsClient{
			responses: map[string]string{
				"provider":       "Google Cloud Platform",
				"region":         "us-central1",
				"stack_name":     "teststack",
				"gcp_project_id": "testproject",
			},
		}
		ec := elicitations.NewController(mockElicitationsClient)
		sm := &mockStacksManager{expectedProvider: cliClient.ProviderGCP}
		_, err := newProvider(ctx, ec, sm, "empty")
		if err != nil && err.Error() != "GCP_PROJECT_ID or CLOUDSDK_CORE_PROJECT must be set for GCP projects" {
			t.Fatalf("getProvider() failed: %v", err)
		}
		if mockCtrl.savedProvider["empty"] != defangv1.Provider_GCP {
			t.Errorf("Expected provider to be saved as GCP, got %v", mockCtrl.savedProvider["empty"])
		}
	})

	t.Run("Should take provider from param without updating saved provider", func(t *testing.T) {
		os.Unsetenv("DIGITALOCEAN_TOKEN")
		os.Unsetenv("DEFANG_PROVIDER")
		mockCtrl.savedProvider = map[string]defangv1.Provider{"empty": defangv1.Provider_AWS}
		RootCmd = FakeRootWithProviderParam("digitalocean")
		ni := global.NonInteractive
		global.NonInteractive = false
		t.Cleanup(func() {
			global.NonInteractive = ni
			mockCtrl.savedProvider = nil
		})

		mockSM := &mockStacksManager{}
		_, err := newProvider(ctx, nil, mockSM, "")
		if err != nil && !strings.HasPrefix(err.Error(), "DIGITALOCEAN_TOKEN must be set") {
			t.Fatalf("getProvider() failed: %v", err)
		}
		if mockCtrl.savedProvider["empty"] != defangv1.Provider_AWS {
			t.Errorf("Expected provider to stay as AWS, but got %v", mockCtrl.savedProvider["empty"])
		}
	})

	t.Run("Should take provider from env aws", func(t *testing.T) {
		t.Setenv("DEFANG_PROVIDER", "aws")
		t.Setenv("AWS_REGION", "us-west-2")
		t.Setenv("DIGITALOCEAN_TOKEN", "") // Clear DO token to avoid interference
		global.Stack.Provider = "aws"      // Manually update to reflect the environment variable
		RootCmd = FakeRootWithProviderParam("")
		sts := aws.StsClient
		aws.StsClient = &mockStsProviderAPI{}
		t.Cleanup(func() {
			aws.StsClient = sts
		})

		mockSM := &mockStacksManager{}
		p, err := newProvider(ctx, nil, mockSM, "empty")
		if err != nil {
			t.Errorf("getProvider() failed: %v", err)
		}
		if _, ok := p.(*aws.ByocAws); !ok {
			t.Errorf("Expected provider to be of type *aws.ByocAws, got %T", p)
		}
	})

	t.Run("Should take provider from env gcp", func(t *testing.T) {
		t.Setenv("DEFANG_PROVIDER", "gcp")
		t.Setenv("GCP_PROJECT_ID", "test_proj_id")
		t.Setenv("DIGITALOCEAN_TOKEN", "") // Clear DO token to avoid interference
		global.Stack.Provider = "gcp"      // Manually update to reflect the environment variable
		RootCmd = FakeRootWithProviderParam("")
		gcpdriver.FindGoogleDefaultCredentials = func(ctx context.Context, scopes ...string) (*google.Credentials, error) {
			return &google.Credentials{
				JSON: []byte(`{"client_email":"test@email.com"}`),
			}, nil
		}

		mockSM := &mockStacksManager{}
		p, err := newProvider(ctx, nil, mockSM, "empty")
		if err != nil {
			t.Errorf("getProvider() failed: %v", err)
		}
		if _, ok := p.(*gcp.ByocGcp); !ok {
			t.Errorf("Expected provider to be of type *aws.ByocGcp, got %T", p)
		}
	})

	t.Run("Should set cd image from canIUse response", func(t *testing.T) {
		t.Setenv("DEFANG_PROVIDER", "aws")
		t.Setenv("AWS_REGION", "us-west-2")
		t.Setenv("DIGITALOCEAN_TOKEN", "") // Clear DO token to avoid interference
		global.Stack.Provider = "aws"      // Manually update to reflect the environment variable
		sts := aws.StsClient
		aws.StsClient = &mockStsProviderAPI{}
		const cdImageTag = "site/registry/repo:tag@sha256:digest"
		mockCtrl.canIUseResponse.CdImage = cdImageTag
		t.Cleanup(func() {
			aws.StsClient = sts
			mockCtrl.canIUseResponse.CdImage = ""
		})

		mockSM := &mockStacksManager{}
		p, err := newProvider(ctx, nil, mockSM, "empty")
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
		t.Setenv("DEFANG_PROVIDER", "aws")
		t.Setenv("AWS_REGION", "us-west-2")
		t.Setenv("DIGITALOCEAN_TOKEN", "") // Clear DO token to avoid interference
		global.Stack.Provider = "aws"      // Manually update to reflect the environment variable
		sts := aws.StsClient
		aws.StsClient = &mockStsProviderAPI{}
		const cdImageTag = "site/registry/repo:tag@sha256:digest"
		const overrideImageTag = "site/override/replaced:tag@sha256:otherdigest"
		t.Setenv("DEFANG_CD_IMAGE", overrideImageTag)
		mockCtrl.canIUseResponse.CdImage = cdImageTag
		t.Cleanup(func() {
			aws.StsClient = sts
			mockCtrl.canIUseResponse.CdImage = ""
		})

		mockSM := &mockStacksManager{}
		p, err := newProvider(ctx, nil, mockSM, "empty")
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
