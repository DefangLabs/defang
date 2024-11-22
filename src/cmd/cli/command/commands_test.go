package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/permissions"
	pkg "github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
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

func TestCommandPermission(t *testing.T) {
	type cmdPermTest struct {
		name      string
		userTier  defangv1.SubscriptionTier
		command   []string
		wantError string
	}
	type cmdPermTests []cmdPermTest

	personalTests := cmdPermTests{
		{
			name:      "PERSONAL - compose up - aws - no permission",
			userTier:  defangv1.SubscriptionTier_PERSONAL,
			command:   []string{"compose", "up", "--provider", "aws", "--dry-run"},
			wantError: "insufficient permissions to perform this action: no compose up to aws",
		},
		{
			name:      "PERSONAL - compose up - defang - no permission",
			userTier:  defangv1.SubscriptionTier_PERSONAL,
			command:   []string{"compose", "up", "--provider", "defang", "--dry-run"},
			wantError: "",
		},
		{
			name:      "PERSONAL - compose down - aws - has permission",
			userTier:  defangv1.SubscriptionTier_PERSONAL,
			command:   []string{"compose", "down", "--provider", "aws", "--dry-run"},
			wantError: "insufficient permissions to perform this action: no compose down to aws",
		},
		{
			name:      "PERSONAL - config set - aws - no permission",
			userTier:  defangv1.SubscriptionTier_PERSONAL,
			command:   []string{"config", "set", "var", "--project-name", "app", "--provider", "aws", "--dry-run"},
			wantError: "insufficient permissions to perform this action: config set on aws",
		},
		{
			name:      "PERSONAL - config rm - aws - no permission",
			userTier:  defangv1.SubscriptionTier_PERSONAL,
			command:   []string{"config", "rm", "var", "--project-name", "app", "--provider", "aws", "--dry-run"},
			wantError: "insufficient permissions to perform this action: config rm on aws",
		},
		{
			name:      "PERSONAL - config rm - defang - has permission",
			userTier:  defangv1.SubscriptionTier_PERSONAL,
			command:   []string{"config", "rm", "var", "--project-name", "app", "--provider", "defang", "--dry-run"},
			wantError: "",
		},
		{
			name:      "PERSONAL - delete service - aws - no permission",
			userTier:  defangv1.SubscriptionTier_PERSONAL,
			command:   []string{"delete", "abc", "--provider", "aws", "--dry-run"},
			wantError: "insufficient permissions to perform this action: delete service on aws",
		},
		// {
		// 	name:      "PERSONAL - send - aws - has permission",
		// 	userTier:  defangv1.SubscriptionTier_PERSONAL,
		// 	command:   []string{"send", "--subject", "subject", "--type", "abc", "--provider", "aws", "--dry-run"},
		// 	wantError: "",
		// },
		// {
		// 	name:      "PERSONAL - token - has permission",
		// 	userTier:  defangv1.SubscriptionTier_PERSONAL,
		// 	command:   []string{"token", "--scope", "abc", "--provider", "aws", "--dry-run"},
		// 	wantError: "",
		// },
	}

	basicTests := cmdPermTests{
		{
			name:      "BASIC - compose up - aws - has permission",
			userTier:  defangv1.SubscriptionTier_BASIC,
			command:   []string{"compose", "up", "--provider", "aws", "--dry-run"},
			wantError: "",
		},
		{
			name:      "BASIC - compose up - aws - has permission",
			userTier:  defangv1.SubscriptionTier_BASIC,
			command:   []string{"compose", "up", "--provider", "aws", "--dry-run"},
			wantError: "",
		},
		{
			name:      "BASIC - config set - aws - has permission",
			userTier:  defangv1.SubscriptionTier_BASIC,
			command:   []string{"config", "set", "var=1234", "--project-name", "app", "--provider", "aws", "--dry-run"},
			wantError: "",
		},
		{
			name:      "BASIC - config rm - aws - has permission",
			userTier:  defangv1.SubscriptionTier_BASIC,
			command:   []string{"config", "rm", "var", "--project-name", "app", "--provider", "aws", "--dry-run"},
			wantError: "",
		},
		{
			name:      "BASIC - config rm - defang - has permission",
			userTier:  defangv1.SubscriptionTier_BASIC,
			command:   []string{"config", "rm", "var", "--project-name", "app", "--provider", "defang", "--dry-run"},
			wantError: "",
		},
	}

	proTests := cmdPermTests{
		{
			name:      "PRO - compose up - aws - has permission",
			userTier:  defangv1.SubscriptionTier_PRO,
			command:   []string{"compose", "up", "--project-name", "app", "--provider", "aws", "--dry-run"},
			wantError: "",
		},
		{
			name:      "PRO - compose down - aws - has permission",
			userTier:  defangv1.SubscriptionTier_PRO,
			command:   []string{"compose", "down", "--project-name", "app", "--dry-run", "--provider", "aws", "--dry-run"},
			wantError: "",
		},
		{
			name:      "PRO - config set - aws - has permission",
			userTier:  defangv1.SubscriptionTier_PRO,
			command:   []string{"config", "set", "var", "--project-name", "app", "--provider", "aws", "--dry-run"},
			wantError: "",
		},
		{
			name:      "PRO - config rm - aws - has permission",
			userTier:  defangv1.SubscriptionTier_PRO,
			command:   []string{"config", "rm", "var", "--project-name", "app", "--provider", "aws", "--dry-run"},
			wantError: "",
		},
		{
			name:      "PRO - config ls - defang - has permission",
			userTier:  defangv1.SubscriptionTier_PRO,
			command:   []string{"config", "rm", "var", "--project-name", "app", "--provider", "defang", "--dry-run"},
			wantError: "",
		},
	}

	for _, testGroup := range []cmdPermTests{proTests, personalTests, basicTests} {
		for _, tt := range testGroup {
			t.Run(tt.name, func(t *testing.T) {
				whoami = &defangv1.WhoAmIResponse{
					Account: "test-account",
					Region:  "us-test-2",
					Tier:    tt.userTier,
				}
				aws.StsClient = stsProviderApi
				pkg.SsmClient = ssmClient

				err := testCommand(tt.command)

				if err != nil && tt.wantError == "" {

					if !strings.Contains(err.Error(), "dry run") && !strings.Contains(err.Error(), "no compose.yaml file found") {
						t.Fatalf("Unexpected error: %v", err)
					}
				}

				if tt.wantError != "" {
					var errNoPermission = permissions.ErrNoPermission(tt.wantError)
					if !errors.As(err, &errNoPermission) {
						t.Fatalf("Expected errNoPermission, got: %v", err)
					}
				}
			})
		}
	}
}
