package command

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/gcp"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	pkg "github.com/DefangLabs/defang/src/pkg/clouds/aws"
	gcpdriver "github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/bufbuild/connect-go"
	connect_go "github.com/bufbuild/connect-go"
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
	allowedToUseProvider bool
	canIUseResponse      defangv1.CanIUseResponse
}

func (m *mockFabricService) CanIUse(ctx context.Context, canUseReq *connect_go.Request[defangv1.CanIUseRequest]) (*connect_go.Response[defangv1.CanIUseResponse], error) {
	if !m.allowedToUseProvider {
		return nil, connect_go.NewError(connect_go.CodePermissionDenied, errors.New("your account does not permit access to use the aws provider. upgrade at https://portal.defang.dev/pricing"))
	}
	return connect_go.NewResponse(&m.canIUseResponse), nil
}

func (m *mockFabricService) GetVersion(context.Context, *connect_go.Request[emptypb.Empty]) (*connect_go.Response[defangv1.Version], error) {
	return connect_go.NewResponse(&defangv1.Version{
		Fabric: "1.0.0-test",
		CliMin: "1.0.0-test",
	}), nil
}

func (m *mockFabricService) CheckToS(context.Context, *connect_go.Request[emptypb.Empty]) (*connect_go.Response[emptypb.Empty], error) {
	return connect_go.NewResponse(&emptypb.Empty{}), nil
}

func (m *mockFabricService) WhoAmI(context.Context, *connect_go.Request[emptypb.Empty]) (*connect_go.Response[defangv1.WhoAmIResponse], error) {
	return connect_go.NewResponse(&defangv1.WhoAmIResponse{
		Tenant:  "default",
		Account: "default",
		Region:  "us-west-2",
		Tier:    defangv1.SubscriptionTier_HOBBY,
	}), nil
}

func (m *mockFabricService) GetSelectedProvider(context.Context, *connect_go.Request[defangv1.GetSelectedProviderRequest]) (*connect_go.Response[defangv1.GetSelectedProviderResponse], error) {
	return connect_go.NewResponse(&defangv1.GetSelectedProviderResponse{
		Provider: defangv1.Provider_AWS,
	}), nil
}

func (m *mockFabricService) SetSelectedProvider(context.Context, *connect_go.Request[defangv1.SetSelectedProviderRequest]) (*connect_go.Response[emptypb.Empty], error) {
	return connect_go.NewResponse(&emptypb.Empty{}), nil
}

func init() {
	SetupCommands(context.Background(), "0.0.0-test")
}

type mockStsProviderAPI struct{}

func (s *mockStsProviderAPI) GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	callIdOutput := sts.GetCallerIdentityOutput{}
	callIdOutput.Account = awssdk.String("123456789012")
	callIdOutput.Arn = awssdk.String("arn:aws:iam::123456789012:user/test")

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
	t.Run("live", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping live test in short mode.")
		}
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
	mockService := &mockFabricService{canIUseResponse: defangv1.CanIUseResponse{}}
	_, handler := defangv1connect.NewFabricControllerHandler(mockService)

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	type cmdPermTest struct {
		name          string
		command       []string
		accessAllowed bool
		wantError     string
	}
	type cmdPermTests []cmdPermTest

	t.Setenv("AWS_REGION", "us-test-2")

	testData := cmdPermTests{
		{
			name:          "compose up - aws - no access",
			command:       []string{"compose", "up", "--project-name=app", "--provider=aws", "--dry-run"},
			accessAllowed: false,
			wantError:     "current subscription tier does not allow this action: no access to use aws provider",
		},
		{
			name:          "compose up - defang - has access",
			command:       []string{"compose", "up", "--provider=defang", "--dry-run"},
			accessAllowed: true,
			wantError:     "",
		},
		{
			name:          "compose down - aws - no access",
			command:       []string{"compose", "down", "--provider=aws", "--dry-run"},
			accessAllowed: false,
			wantError:     "current subscription tier does not allow this action: no access to use aws provider",
		},
		{
			name:          "config set - aws - no access",
			command:       []string{"config", "set", "var", "--project-name=app", "--provider=aws", "--dry-run"},
			accessAllowed: false,
			wantError:     "current subscription tier does not allow this action: no access to use aws provider",
		},
		{
			name:          "config rm - aws - no access",
			command:       []string{"config", "rm", "var", "--project-name=app", "--provider=aws", "--dry-run"},
			accessAllowed: false,
			wantError:     "current subscription tier does not allow this action: no access to use aws provider",
		},
		{
			name:          "config rm - defang - has access",
			command:       []string{"config", "rm", "var", "--project-name=app", "--provider=defang", "--dry-run"},
			accessAllowed: true,
			wantError:     "",
		},
		{
			name:          "delete service - aws - no access",
			command:       []string{"delete", "abc", "--provider=aws", "--dry-run"},
			accessAllowed: false,
			wantError:     "current subscription tier does not allow this action: no access to use aws provider",
		},
	}

	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			aws.StsClient = &mockStsProviderAPI{}
			pkg.SsmClientOverride = &MockSsmClient{}
			mockService.allowedToUseProvider = tt.accessAllowed

			err := testCommand(tt.command, server.URL)

			if err != nil && tt.wantError == "" {
				if !strings.Contains(err.Error(), "dry run") && !strings.Contains(err.Error(), "no compose.yaml file found") {
					t.Fatalf("Unexpected error: %v", err)
				}
			}

			if tt.wantError != "" {
				var errNoPermission = ErrNoPermission(tt.wantError)
				if !errors.As(err, &errNoPermission) || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("Expected errNoPermission, got: %v", err)
				}
			}
		})
	}
}

type MockFabricControllerClient struct {
	defangv1connect.FabricControllerClient
	canIUseResponse defangv1.CanIUseResponse
	savedProvider   defangv1.Provider
}

func (m *MockFabricControllerClient) CanIUse(context.Context, *connect_go.Request[defangv1.CanIUseRequest]) (*connect_go.Response[defangv1.CanIUseResponse], error) {
	return connect.NewResponse(&m.canIUseResponse), nil
}

func (m *MockFabricControllerClient) GetServices(context.Context, *connect_go.Request[defangv1.GetServicesRequest]) (*connect_go.Response[defangv1.GetServicesResponse], error) {
	return connect.NewResponse(&defangv1.GetServicesResponse{}), nil
}

func (m *MockFabricControllerClient) GetSelectedProvider(context.Context, *connect_go.Request[defangv1.GetSelectedProviderRequest]) (*connect_go.Response[defangv1.GetSelectedProviderResponse], error) {
	return connect.NewResponse(&defangv1.GetSelectedProviderResponse{
		Provider: m.savedProvider,
	}), nil
}

func TestGetProvider(t *testing.T) {
	mockClient := cliClient.GrpcClient{Client: &MockFabricControllerClient{
		canIUseResponse: defangv1.CanIUseResponse{},
	}}
	client = mockClient
	loader := cliClient.MockLoader{Project: &compose.Project{Name: "empty"}}

	t.Run("Nil loader auto provider non-interactive should load playground provider", func(t *testing.T) {
		ctx := context.Background()
		providerID = "auto"
		os.Unsetenv("DEFANG_PROVIDER")

		p, err := getProvider(ctx, nil)
		if err != nil {
			t.Fatalf("getProvider() failed: %v", err)
		}
		if _, ok := p.(*cliClient.PlaygroundProvider); !ok {
			t.Errorf("Expected provider to be of type *cliClient.PlaygroundProvider, got %T", p)
		}
	})

	t.Run("Auto provider should get provider from client", func(t *testing.T) {
		ctx := context.Background()
		providerID = "auto"
		os.Unsetenv("DEFANG_PROVIDER")
		t.Setenv("AWS_REGION", "us-west-2")
		mockCtrl, _ := client.Client.(*MockFabricControllerClient)
		mockCtrl.savedProvider = defangv1.Provider_AWS
		RootCmd.ResetFlags() // TODO: This should not be needed, but seems other tests messes up RootCmd.PersistentFlags()

		ni := nonInteractive
		sts := aws.StsClient
		aws.StsClient = &mockStsProviderAPI{}
		nonInteractive = false
		t.Cleanup(func() {
			nonInteractive = ni
			aws.StsClient = sts
			mockCtrl.savedProvider = defangv1.Provider_PROVIDER_UNSPECIFIED
		})

		p, err := getProvider(ctx, loader)
		if err != nil {
			t.Fatalf("getProvider() failed: %v", err)
		}
		if _, ok := p.(*aws.ByocAws); !ok {
			t.Errorf("Expected provider to be of type *aws.ByocAws, got %T", p)
		}
	})

	t.Run("Should take provider from env aws", func(t *testing.T) {
		ctx := context.Background()
		t.Setenv("DEFANG_PROVIDER", "aws")
		t.Setenv("AWS_REGION", "us-west-2")
		sts := aws.StsClient
		aws.StsClient = &mockStsProviderAPI{}
		t.Cleanup(func() {
			aws.StsClient = sts
		})

		p, err := getProvider(ctx, loader)
		if err != nil {
			t.Errorf("getProvider() failed: %v", err)
		}
		if _, ok := p.(*aws.ByocAws); !ok {
			t.Errorf("Expected provider to be of type *aws.ByocAws, got %T", p)
		}
	})

	t.Run("Should take provider from env gcp", func(t *testing.T) {
		ctx := context.Background()
		t.Setenv("DEFANG_PROVIDER", "gcp")
		t.Setenv("GCP_PROJECT_ID", "test_proj_id")
		gcpdriver.FindGoogleDefaultCredentials = func(ctx context.Context, scopes ...string) (*google.Credentials, error) {
			return &google.Credentials{
				JSON: []byte(`{"client_email":"test@email.com"}`),
			}, nil
		}

		p, err := getProvider(ctx, loader)
		if err != nil {
			t.Errorf("getProvider() failed: %v", err)
		}
		if _, ok := p.(*gcp.ByocGcp); !ok {
			t.Errorf("Expected provider to be of type *aws.ByocGcp, got %T", p)
		}
	})

	t.Run("Should set cd image from canIUse response", func(t *testing.T) {
		ctx := context.Background()
		t.Setenv("DEFANG_PROVIDER", "aws")
		t.Setenv("AWS_REGION", "us-west-2")
		sts := aws.StsClient
		aws.StsClient = &mockStsProviderAPI{}
		const cdImageTag = "site/registry/repo:tag@sha256:digest"
		mockCtrl, _ := client.Client.(*MockFabricControllerClient)
		mockCtrl.canIUseResponse.CdImage = cdImageTag
		t.Cleanup(func() {
			aws.StsClient = sts
			mockCtrl.canIUseResponse.CdImage = ""
		})

		p, err := getProvider(ctx, loader)
		if err != nil {
			t.Errorf("getProvider() failed: %v", err)
		}
		if awsProvider, ok := p.(*aws.ByocAws); !ok {
			t.Errorf("Expected provider to be of type *aws.ByocAws, got %T", p)
		} else {
			if awsProvider.CDImage != cdImageTag {
				t.Errorf("Expected cd image tag to be %s, got %s", cdImageTag, awsProvider.CDImage)
			}
		}
	})

	t.Run("Can override cd image from environment variable", func(t *testing.T) {
		ctx := context.Background()
		t.Setenv("DEFANG_PROVIDER", "aws")
		t.Setenv("AWS_REGION", "us-west-2")
		sts := aws.StsClient
		aws.StsClient = &mockStsProviderAPI{}
		const cdImageTag = "site/registry/repo:tag@sha256:digest"
		const overrideImageTag = "site/override/replaced:tag@sha256:otherdigest"
		t.Setenv("DEFANG_CD_IMAGE", overrideImageTag)
		mockCtrl, _ := client.Client.(*MockFabricControllerClient)
		mockCtrl.canIUseResponse.CdImage = cdImageTag
		t.Cleanup(func() {
			aws.StsClient = sts
			mockCtrl.canIUseResponse.CdImage = ""
		})

		p, err := getProvider(ctx, loader)
		if err != nil {
			t.Errorf("getProvider() failed: %v", err)
		}
		if awsProvider, ok := p.(*aws.ByocAws); !ok {
			t.Errorf("Expected provider to be of type *aws.ByocAws, got %T", p)
		} else {
			if awsProvider.CDImage != overrideImageTag {
				t.Errorf("Expected cd image tag to be %s, got %s", cdImageTag, awsProvider.CDImage)
			}
		}
	})
}
