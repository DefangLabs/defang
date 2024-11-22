package command

import (
	"context"
	"errors"
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func TestVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	err := testCommand([]string{"version"})
	if err != nil {
		t.Fatalf("Version() failed: %v", err)
	}
}

func testCommand(args []string) error {
	ctx := context.Background()
	SetupCommands(ctx, "test")
	RootCmd.SetArgs(args)
	return RootCmd.ExecuteContext(ctx)
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

func TestCommandPermission(t *testing.T) {
	whoami = &defangv1.WhoAmIResponse{
		Account: "test-account",
		Region:  "us-test-2",
		Tier:    defangv1.SubscriptionTier_PERSONAL,
	}
	aws.StsClient = stsProviderApi
	err := testCommand([]string{"compose", "up", "--provider", "aws"})

	var errNoPermission *ErrNoPermission
	if !errors.As(err, &errNoPermission) {
		t.Fatalf("Expected errNoPermission, got: %v", err)
	}
}
