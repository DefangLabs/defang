package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/gating"
	pkg "github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/store"
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

func TestCommandGates(t *testing.T) {
	type cmdPermTest struct {
		name         string
		userTier     defangv1.SubscriptionTier
		command      []string
		lastProvider cliClient.ProviderID
		wantError    string
	}
	type cmdPermTests []cmdPermTest

	hobbyTests := cmdPermTests{
		{
			name:         "HOBBY - compose up - aws - no access",
			userTier:     defangv1.SubscriptionTier_HOBBY,
			command:      []string{"compose", "up", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "current tier does not allow this action: no access to use aws provider",
		},
		{
			name:         "HOBBY - compose up - defang - no access",
			userTier:     defangv1.SubscriptionTier_HOBBY,
			command:      []string{"compose", "up", "--provider=defang", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "",
		},
		{
			name:         "HOBBY - compose down - aws - has access",
			userTier:     defangv1.SubscriptionTier_HOBBY,
			command:      []string{"compose", "down", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "current tier does not allow this action: no access to use aws provider",
		},
		{
			name:         "HOBBY - config set - aws - no access",
			userTier:     defangv1.SubscriptionTier_HOBBY,
			command:      []string{"config", "set", "var", "--project-name=app", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "current tier does not allow this action: no access to use aws provider",
		},
		{
			name:         "HOBBY - config rm - aws - no access",
			userTier:     defangv1.SubscriptionTier_HOBBY,
			command:      []string{"config", "rm", "var", "--project-name=app", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "current tier does not allow this action: no access to use aws provider",
		},
		{
			name:         "HOBBY - config rm - defang - has access",
			userTier:     defangv1.SubscriptionTier_HOBBY,
			command:      []string{"config", "rm", "var", "--project-name=app", "--provider=defang", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "",
		},
		{
			name:         "HOBBY - delete service - aws - no access",
			userTier:     defangv1.SubscriptionTier_HOBBY,
			command:      []string{"delete", "abc", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "current tier does not allow this action: no access to use aws provider",
		},
		// {
		// 	name:         "HOBBY - send - aws - has access",
		// 	userTier:     defangv1.SubscriptionTier_HOBBY,
		// 	command:      []string{"send", "--subject=subject", "--type=abc", "--provider=aws", "--dry-run"},
		// 	lastProvider: cliClient.ProviderAuto,
		// 	wantError:    "",
		// },
		// {
		// 	name:         "HOBBY - token - has access",
		// 	userTier:     defangv1.SubscriptionTier_HOBBY,
		// 	command:      []string{"token", "--scope=abc", "--provider=aws", "--dry-run"},
		// 	lastProvider: cliClient.ProviderAuto,
		// 	wantError:    "",
		// },
	}

	personalTests := cmdPermTests{
		{
			name:         "PERSONAL - compose up - aws - has access",
			userTier:     defangv1.SubscriptionTier_PERSONAL,
			command:      []string{"compose", "up", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "",
		},
		{
			name:         "PERSONAL - compose up - change provider - no access",
			userTier:     defangv1.SubscriptionTier_PERSONAL,
			command:      []string{"compose", "up", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderDO,
			wantError:    "current tier does not allow this action: basic tier only supports one cloud provider at a time",
		},
		{
			name:         "PERSONAL - compose up - aws - has access",
			userTier:     defangv1.SubscriptionTier_PERSONAL,
			command:      []string{"compose", "up", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "",
		},
		{
			name:         "PERSONAL - config set - aws - has access",
			userTier:     defangv1.SubscriptionTier_PERSONAL,
			command:      []string{"config", "set", "var=1234", "--project-name=app", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "",
		},
		{
			name:         "PERSONAL - config rm - aws - has access",
			userTier:     defangv1.SubscriptionTier_PERSONAL,
			command:      []string{"config", "rm", "var", "--project-name=app", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "",
		},
		{
			name:         "PERSONAL - config rm - defang - has access",
			userTier:     defangv1.SubscriptionTier_PERSONAL,
			command:      []string{"config", "rm", "var", "--project-name=app", "--provider=defang", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "",
		},
	}

	proTests := cmdPermTests{
		{
			name:         "PRO - compose up - aws - has access",
			userTier:     defangv1.SubscriptionTier_PRO,
			command:      []string{"compose", "up", "--project-name=app", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "",
		},
		{
			name:         "PRO - compose down - aws - has access",
			userTier:     defangv1.SubscriptionTier_PRO,
			command:      []string{"compose", "down", "--project-name=app", "--dry-run", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "",
		},
		{
			name:         "PRO - config set - aws - has access",
			userTier:     defangv1.SubscriptionTier_PRO,
			command:      []string{"config", "set", "var", "--project-name=app", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "",
		},
		{
			name:         "PRO - config rm - aws - has access",
			userTier:     defangv1.SubscriptionTier_PRO,
			command:      []string{"config", "rm", "var", "--project-name=app", "--provider=aws", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "",
		},
		{
			name:         "PRO - config ls - defang - has access",
			userTier:     defangv1.SubscriptionTier_PRO,
			command:      []string{"config", "rm", "var", "--project-name=app", "--provider=defang", "--dry-run"},
			lastProvider: cliClient.ProviderAuto,
			wantError:    "",
		},
	}

	for _, testGroup := range []cmdPermTests{proTests, hobbyTests, personalTests} {
		for _, tt := range testGroup {
			t.Run(tt.name, func(t *testing.T) {
				store.ReadOnlyUserWhoAmI = false
				store.SetUserWhoAmI(&defangv1.WhoAmIResponse{
					Account: "test-account",
					Region:  "us-test-2",
					Tier:    tt.userTier,
				})
				store.ReadOnlyUserWhoAmI = true
				aws.StsClient = stsProviderApi
				pkg.SsmClientOverride = ssmClient
				overrideLastUseProviderID = tt.lastProvider

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
