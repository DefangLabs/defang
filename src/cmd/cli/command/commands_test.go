package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/gating"
	pkg "github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type MockSsmClient struct {
	pkg.SsmParametersAPI
}

type MockGrpcClientApi struct {
	GrpcClientApi
}

func (m *MockSsmClient) PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	return &ssm.PutParameterOutput{}, nil
}

func (m *MockSsmClient) DeleteParameters(ctx context.Context, params *ssm.DeleteParametersInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParametersOutput, error) {
	return &ssm.DeleteParametersOutput{
		DeletedParameters: []string{"var"},
	}, nil
}

var mockCanUseProviderResponse error = nil

var mockWhoAmIResponse = &defangv1.WhoAmIResponse{
	Tenant:  "default",
	Account: "default",
	Region:  "us-west-2",
	Tier:    defangv1.SubscriptionTier_HOBBY,
}

func (m *MockGrpcClientApi) CanUseProvider(ctx context.Context, canUseReq *defangv1.CanUseProviderRequest) error {
	return mockCanUseProviderResponse
}

func (m *MockGrpcClientApi) GetVersions(ctx context.Context) (*defangv1.Version, error) {
	return &defangv1.Version{
		Fabric: "1.0.0",
		CliMin: "1.0.0",
	}, nil
}
func (m *MockGrpcClientApi) CheckLoginAndToS(context.Context) error {
	return nil
}

func (m *MockGrpcClientApi) WhoAmI(context.Context) (*defangv1.WhoAmIResponse, error) {
	return mockWhoAmIResponse, nil
}

func (m *MockGrpcClientApi) GetSelectedProvider(context.Context, *defangv1.GetSelectedProviderRequest) (*defangv1.GetSelectedProviderResponse, error) {
	return &defangv1.GetSelectedProviderResponse{
		Provider: defangv1.Provider_AWS,
	}, nil
}

func (m *MockGrpcClientApi) SetSelectedProvider(context.Context, *defangv1.SetSelectedProviderRequest) error {
	return nil
}

var ctx = context.Background()

func init() {
	SetupCommands(ctx, "version")
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

var stsProviderApi aws.StsProviderAPI = &mockStsProviderAPI{}
var ssmClient = &MockSsmClient{}

func testCommand(args []string) error {
	RootCmd.SetArgs(args)
	return RootCmd.ExecuteContext(ctx)
}

func TestVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	err := testCommand([]string{"version"})
	if err != nil {
		t.Fatalf("Version() failed: %v", err)
	}
}

func TestCommandGates(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	type cmdPermTest struct {
		name          string
		command       []string
		accessAllowed bool
		wantError     string
	}
	type cmdPermTests []cmdPermTest

	localClient = &MockGrpcClientApi{}

	testData := cmdPermTests{
		{
			name:          "compose up - aws - no access",
			command:       []string{"compose", "up", "--project-name=app", "--provider=aws", "--dry-run"},
			accessAllowed: false,
			wantError:     "current tier does not allow this action: no access to use aws provider",
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
			wantError:     "current tier does not allow this action: no access to use aws provider",
		},
		{
			name:          "config set - aws - no access",
			command:       []string{"config", "set", "var", "--project-name=app", "--provider=aws", "--dry-run"},
			accessAllowed: false,
			wantError:     "current tier does not allow this action: no access to use aws provider",
		},
		{
			name:          "config rm - aws - no access",
			command:       []string{"config", "rm", "var", "--project-name=app", "--provider=aws", "--dry-run"},
			accessAllowed: false,
			wantError:     "current tier does not allow this action: no access to use aws provider",
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
			wantError:     "current tier does not allow this action: no access to use aws provider",
		},
	}

	for _, testGroup := range []cmdPermTests{testData} {
		for _, tt := range testGroup {
			t.Run(tt.name, func(t *testing.T) {
				aws.StsClient = stsProviderApi
				pkg.SsmClientOverride = ssmClient
				mockCanUseProviderResponse = nil
				if !tt.accessAllowed {
					mockCanUseProviderResponse = errors.New("no access")
				}

				err := testCommand(tt.command)

				if err != nil && tt.wantError == "" {
					if !strings.Contains(err.Error(), "dry run") && !strings.Contains(err.Error(), "no compose.yaml file found") {
						t.Fatalf("Unexpected error: %v", err)
					}
				}

				if tt.wantError != "" {
					var errNoPermission = gating.ErrNoPermission(tt.wantError)
					if !errors.As(err, &errNoPermission) || !strings.Contains(err.Error(), tt.wantError) {
						t.Fatalf("Expected errNoPermission, got: %v", err)
					}
				}
			})
		}
	}
}
