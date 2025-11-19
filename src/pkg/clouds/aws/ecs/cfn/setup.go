package cfn

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	common "github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/region"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfnTypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/ptr"
)

type AwsEcsCfn struct {
	ecs.AwsEcs
	stackName string
}

// var _ types.Driver = (*AwsEcs)(nil)

const stackTimeout = time.Minute * 3

func OptionVPCAndSubnetID(ctx context.Context, vpcID, subnetID string) func(types.Driver) error {
	return func(d types.Driver) error {
		if ecs, ok := d.(*AwsEcsCfn); ok {
			return ecs.PopulateVPCandSubnetID(ctx, vpcID, subnetID)
		}
		return errors.New("only AwsEcs driver supports VPC ID and Subnet ID option")
	}
}

func New(stack string, region region.Region) *AwsEcsCfn {
	if stack == "" {
		panic("stack must be set")
	}
	return &AwsEcsCfn{
		stackName: stack,
		AwsEcs: ecs.AwsEcs{
			Aws:  common.Aws{Region: region},
			Spot: true,
		},
	}
}

func (a *AwsEcsCfn) newClient(ctx context.Context) (*cloudformation.Client, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	return cloudformation.NewFromConfig(cfg), nil
}

func (a *AwsEcsCfn) updateStackAndWait(ctx context.Context, templateBody string) error {
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	// Check the template version first, to avoid updating to an outdated template; TODO: can we use StackPolicy/Conditions instead?
	if dso, err := cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: &a.stackName}); err == nil && len(dso.Stacks) == 1 {
		for _, output := range dso.Stacks[0].Outputs {
			if *output.OutputKey == OutputsTemplateVersion {
				deployedRev, _ := strconv.Atoi(*output.OutputValue)
				if deployedRev > TemplateRevision {
					return fmt.Errorf("CloudFormation stack %s is newer than the current template: update the CLI", a.stackName)
				}
			}
		}
	}

	uso, err := cfn.UpdateStack(ctx, &cloudformation.UpdateStackInput{
		Capabilities: []cfnTypes.Capability{cfnTypes.CapabilityCapabilityNamedIam},
		StackName:    ptr.String(a.stackName),
		TemplateBody: ptr.String(templateBody),
	})
	if err != nil {
		// Go SDK doesn't have --no-fail-on-empty-changeset; ignore ValidationError: No updates are to be performed.
		var apiError smithy.APIError
		if ok := errors.As(err, &apiError); ok && apiError.ErrorCode() == "ValidationError" && apiError.ErrorMessage() == "No updates are to be performed." {
			return a.FillOutputs(ctx)
		}
		// TODO: handle UPDATE_COMPLETE_CLEANUP_IN_PROGRESS
		return err // might call createStackAndWait depending on the error
	}

	fmt.Println("Waiting for CloudFormation stack", a.stackName, "to be updated...") // TODO: verbose only
	dso, err := cloudformation.NewStackUpdateCompleteWaiter(cfn, update1s).WaitForOutput(ctx, &cloudformation.DescribeStacksInput{
		StackName: uso.StackId,
	}, stackTimeout)
	if err != nil {
		return fmt.Errorf("failed to update CloudFormation stack: check the CloudFormation console (https://%s.console.aws.amazon.com/cloudformation/home) for the %q stack to learn more: %w", a.AwsEcs.Region, a.stackName, err)
	}
	return a.fillWithOutputs(dso)
}

func (a *AwsEcsCfn) createStackAndWait(ctx context.Context, templateBody string) error {
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	_, err = cfn.CreateStack(ctx, &cloudformation.CreateStackInput{
		Capabilities:                []cfnTypes.Capability{cfnTypes.CapabilityCapabilityNamedIam},
		EnableTerminationProtection: ptr.Bool(true),
		OnFailure:                   cfnTypes.OnFailureDelete,
		StackName:                   ptr.String(a.stackName),
		TemplateBody:                ptr.String(templateBody),
	})
	if err != nil {
		// Ignore AlreadyExistsException; return all other errors
		var alreadyExists *cfnTypes.AlreadyExistsException
		if !errors.As(err, &alreadyExists) {
			return err
		}
	}

	fmt.Println("Waiting for CloudFormation stack", a.stackName, "to be created...") // TODO: verbose only
	dso, err := cloudformation.NewStackCreateCompleteWaiter(cfn, create1s).WaitForOutput(ctx, &cloudformation.DescribeStacksInput{
		StackName: ptr.String(a.stackName),
	}, stackTimeout)
	if err != nil {
		return fmt.Errorf("failed to create CloudFormation stack: check the CloudFormation console (https://%s.console.aws.amazon.com/cloudformation/home) for the %q stack to learn more: %w", a.AwsEcs.Region, a.stackName, err)
	}
	return a.fillWithOutputs(dso)
}

func (a *AwsEcsCfn) SetUp(ctx context.Context, containers []types.Container) error {
	tmpl, err := createTemplate(a.stackName, containers, TemplateOverrides{VpcID: a.VpcID, Spot: a.Spot})
	if err != nil {
		return fmt.Errorf("failed to create CloudFormation template: %w", err)
	}

	template, err := tmpl.YAML()
	if err != nil {
		return fmt.Errorf("failed to marshal CloudFormation template as YAML: %w", err)
	}

	// Upsert
	if err := a.updateStackAndWait(ctx, string(template)); err != nil {
		// Check if the stack doesn't exist; if so, create it, otherwise return the error
		var apiError smithy.APIError
		if ok := errors.As(err, &apiError); !ok || (apiError.ErrorCode() != "ValidationError") || !strings.HasSuffix(apiError.ErrorMessage(), "does not exist") {
			return err
		}
		return a.createStackAndWait(ctx, string(template))
	}
	return nil
}

type ErrStackNotFoundException = cfnTypes.StackNotFoundException

func (a *AwsEcsCfn) FillOutputs(ctx context.Context) error {
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	// NOTE: this always returns the latest outputs, not the ones from the recent update
	dso, err := cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: ptr.String(a.stackName),
	})
	if err != nil {
		// Check if the stack doesn't exist (ValidationError); if so, return a nice error; https://github.com/aws/aws-sdk-go-v2/issues/2296
		var ae smithy.APIError
		if errors.As(err, &ae) && ae.ErrorCode() == "ValidationError" {
			return &ErrStackNotFoundException{Message: ptr.String(ae.ErrorMessage())}
		}
		return err
	}

	return a.fillWithOutputs(dso)
}

func (a *AwsEcsCfn) fillWithOutputs(dso *cloudformation.DescribeStacksOutput) error {
	if len(dso.Stacks) != 1 {
		return fmt.Errorf("expected 1 CloudFormation stack, got %d", len(dso.Stacks))
	}
	for _, output := range dso.Stacks[0].Outputs {
		switch *output.OutputKey {
		case OutputsSubnetID:
			// Only set the SubNetID if it's not already set; this allows the user to override the subnet
			if a.SubNetID == "" {
				a.SubNetID = *output.OutputValue
			}
		case OutputsDefaultSecurityGroupID:
			a.DefaultSecurityGroupID = *output.OutputValue
		case OutputsTaskDefArn:
			a.TaskDefARN = *output.OutputValue
		case OutputsClusterName:
			a.ClusterName = *output.OutputValue
		case OutputsLogGroupARN:
			a.LogGroupARN = *output.OutputValue
		case OutputsSecurityGroupID:
			a.SecurityGroupID = *output.OutputValue
		case OutputsBucketName:
			a.BucketName = *output.OutputValue
		}
	}

	return nil
}

func (a *AwsEcsCfn) Run(ctx context.Context, env map[string]string, cmd ...string) (ecs.TaskArn, error) {
	if err := a.FillOutputs(ctx); err != nil {
		return nil, err
	}

	return a.AwsEcs.Run(ctx, env, cmd...)
}

func (a *AwsEcsCfn) Tail(ctx context.Context, taskArn ecs.TaskArn) error {
	if err := a.FillOutputs(ctx); err != nil {
		return err
	}
	return a.AwsEcs.Tail(ctx, taskArn)
}

func (a *AwsEcsCfn) Stop(ctx context.Context, taskArn ecs.TaskArn) error {
	if err := a.FillOutputs(ctx); err != nil {
		return err
	}
	return a.AwsEcs.Stop(ctx, taskArn)
}

func (a *AwsEcsCfn) GetInfo(ctx context.Context, taskArn ecs.TaskArn) (*types.TaskInfo, error) {
	if err := a.FillOutputs(ctx); err != nil {
		return nil, err
	}
	return a.AwsEcs.Info(ctx, taskArn)
}

func (a *AwsEcsCfn) TearDown(ctx context.Context) error {
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	// Disable termination protection before deleting the stack
	if _, err := cfn.UpdateTerminationProtection(ctx, &cloudformation.UpdateTerminationProtectionInput{
		StackName:                   ptr.String(a.stackName),
		EnableTerminationProtection: ptr.Bool(false),
	}); err != nil {
		fmt.Printf("Failed to disable termination protection for CloudFormation stack %s: %v\n", a.stackName, err)
	}
	_, err = cfn.DeleteStack(ctx, &cloudformation.DeleteStackInput{
		StackName: ptr.String(a.stackName),
		// RetainResources: []string{"Bucket"}, only when the stack is in the DELETE_FAILED state
	})
	if err != nil {
		return err
	}

	fmt.Println("Waiting for CloudFormation stack", a.stackName, "to be deleted...") // TODO: verbose only
	return cloudformation.NewStackDeleteCompleteWaiter(cfn, delete1s).Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: ptr.String(a.stackName),
	}, stackTimeout)
}
