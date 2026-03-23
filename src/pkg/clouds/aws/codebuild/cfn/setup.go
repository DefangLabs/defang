package cfn

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	common "github.com/DefangLabs/defang/src/pkg/clouds/aws"
	awscodebuild "github.com/DefangLabs/defang/src/pkg/clouds/aws/codebuild"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/region"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfnTypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/ptr"
)

type AwsCfn struct {
	awscodebuild.AwsCodeBuild
	stackName string
}

const stackTimeout = time.Minute * 3

func New(stack string, region region.Region) *AwsCfn {
	if stack == "" {
		panic("stack must be set")
	}
	return &AwsCfn{
		stackName: stack,
		AwsCodeBuild: awscodebuild.AwsCodeBuild{
			Aws:          common.Aws{Region: region},
			RetainBucket: true,
		},
	}
}

func (a *AwsCfn) newClient(ctx context.Context) (*cloudformation.Client, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	return cloudformation.NewFromConfig(cfg), nil
}

func (a *AwsCfn) updateStackAndWait(ctx context.Context, templateBody string, force bool, parameters []cfnTypes.Parameter) error {
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	// Check the template version first, to avoid updating to an outdated template; TODO: can we use StackPolicy/Conditions instead?
	var deployedRev int
	// TODO: should check all regions
	if dso, err := cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: &a.stackName}); err == nil && len(dso.Stacks) == 1 {
		for _, output := range dso.Stacks[0].Outputs {
			if *output.OutputKey == OutputsTemplateVersion {
				deployedRev, _ = strconv.Atoi(*output.OutputValue)
				if deployedRev > TemplateRevision && !force {
					return fmt.Errorf("This version of the CLI expects CloudFormation template v%d, but the deployed %s stack is v%d: please update the CLI", TemplateRevision, a.stackName, deployedRev)
				}
			}
		}
		// Remove any parameters that have UsePreviousValue set, but are not in the previously deployed stack
		previousParams := make(map[string]bool)
		for _, p := range dso.Stacks[0].Parameters {
			previousParams[*p.ParameterKey] = true
		}
		parameters = slices.DeleteFunc(parameters, func(p cfnTypes.Parameter) bool {
			return p.UsePreviousValue != nil && *p.UsePreviousValue && !previousParams[*p.ParameterKey]
		})
	}

	uso, err := cfn.UpdateStack(ctx, &cloudformation.UpdateStackInput{
		Capabilities: []cfnTypes.Capability{cfnTypes.CapabilityCapabilityNamedIam},
		Parameters:   parameters,
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

	term.Info("Waiting for CloudFormation stack", a.stackName, "to be updated...") // TODO: verbose only
	dso, err := cloudformation.NewStackUpdateCompleteWaiter(cfn, update1s).WaitForOutput(ctx, &cloudformation.DescribeStacksInput{
		StackName: uso.StackId,
	}, stackTimeout)
	if err != nil {
		var extra string
		if deployedRev == 3 && TemplateRevision == 4 {
			// Upgrade from 3->4 involves deleting the VPC, which will fail / timeout when the VPC, or any SG, is in use.
			extra = " check if the VPC or any security group is still in use and delete them manually before retrying the update;"
		}
		return fmt.Errorf("failed to update CloudFormation stack:%s check the CloudFormation console (https://%s.console.aws.amazon.com/cloudformation/home) for the %q stack to learn more: %w", extra, a.AwsCodeBuild.Region, a.stackName, err)
	}
	return a.fillWithOutputs(dso)
}

func (a *AwsCfn) createStackAndWait(ctx context.Context, templateBody string, parameters []cfnTypes.Parameter) error {
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	_, err = cfn.CreateStack(ctx, &cloudformation.CreateStackInput{
		Capabilities:                []cfnTypes.Capability{cfnTypes.CapabilityCapabilityNamedIam},
		EnableTerminationProtection: ptr.Bool(true),
		OnFailure:                   cfnTypes.OnFailureDelete,
		Parameters:                  parameters,
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

	term.Info("Waiting for CloudFormation stack", a.stackName, "to be created...") // TODO: verbose only
	dso, err := cloudformation.NewStackCreateCompleteWaiter(cfn, create1s).WaitForOutput(ctx, &cloudformation.DescribeStacksInput{
		StackName: ptr.String(a.stackName),
	}, stackTimeout)
	if err != nil {
		return fmt.Errorf("failed to create CloudFormation stack: check the CloudFormation console (https://%s.console.aws.amazon.com/cloudformation/home) for the %q stack to learn more: %w", a.AwsCodeBuild.Region, a.stackName, err)
	}
	return a.fillWithOutputs(dso)
}

func (a *AwsCfn) SetUp(ctx context.Context, force bool) (bool, error) {
	template, err := CreateTemplate(a.stackName)
	if err != nil {
		return false, fmt.Errorf("failed to create CloudFormation template: %w", err)
	}

	templateBody, err := template.YAML()
	if err != nil {
		return false, err
	}

	// Set parameter values based on current configuration or leave nil to use previous values or defaults
	var parameters []cfnTypes.Parameter
	for key := range template.Parameters {
		param := cfnTypes.Parameter{
			ParameterKey:     ptr.String(key),
			UsePreviousValue: ptr.Bool(true),
		}
		if key == ParamsRetainBucket {
			param.ParameterValue = ptr.String(strconv.FormatBool(a.RetainBucket))
			param.UsePreviousValue = nil
		}
		parameters = append(parameters, param)
	}

	return a.upsertStackAndWait(ctx, templateBody, force, parameters...)
}

func (a *AwsCfn) upsertStackAndWait(ctx context.Context, templateBody []byte, force bool, parameters ...cfnTypes.Parameter) (bool, error) {
	// Upsert with parameters
	if err := a.updateStackAndWait(ctx, string(templateBody), force, parameters); err != nil {
		// Check if the stack doesn't exist; if so, create it, otherwise return the error
		var apiError smithy.APIError
		if ok := errors.As(err, &apiError); !ok || (apiError.ErrorCode() != "ValidationError") || !strings.HasSuffix(apiError.ErrorMessage(), "does not exist") {
			return false, err
		}
		return true, a.createStackAndWait(ctx, string(templateBody), parameters)
	}
	return false, nil
}

type ErrStackNotFoundException = cfnTypes.StackNotFoundException

func (a *AwsCfn) FillOutputs(ctx context.Context) error {
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

func (a *AwsCfn) fillWithOutputs(dso *cloudformation.DescribeStacksOutput) error {
	if len(dso.Stacks) != 1 {
		return fmt.Errorf("expected 1 CloudFormation stack, got %d", len(dso.Stacks))
	}

	a.LogGroupARN = ""
	a.BucketName = ""
	a.CIRoleARN = ""
	a.ProjectName = ""
	for _, output := range dso.Stacks[0].Outputs {
		switch *output.OutputKey {
		case OutputsLogGroupARN:
			a.LogGroupARN = *output.OutputValue
		case OutputsBucketName:
			a.BucketName = *output.OutputValue
		case OutputsCIRoleARN:
			a.CIRoleARN = *output.OutputValue
		case OutputsCodeBuildProjectName:
			a.ProjectName = *output.OutputValue
		}
	}

	if a.AccountID == "" && a.LogGroupARN != "" {
		a.AccountID = common.GetAccountID(a.LogGroupARN)
	}
	return nil
}

func (a *AwsCfn) TearDown(ctx context.Context) error {
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	// Disable termination protection before deleting the stack
	if _, err := cfn.UpdateTerminationProtection(ctx, &cloudformation.UpdateTerminationProtectionInput{
		StackName:                   ptr.String(a.stackName),
		EnableTerminationProtection: ptr.Bool(false),
	}); err != nil {
		term.Warnf("Failed to disable termination protection for CloudFormation stack %s: %v\n", a.stackName, err)
	}
	_, err = cfn.DeleteStack(ctx, &cloudformation.DeleteStackInput{
		StackName: ptr.String(a.stackName),
		// RetainResources: []string{"Bucket"}, only when the stack is in the DELETE_FAILED state
	})
	if err != nil {
		return err
	}

	term.Info("Waiting for CloudFormation stack", a.stackName, "to be deleted...") // TODO: verbose only
	return cloudformation.NewStackDeleteCompleteWaiter(cfn, delete1s).Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: ptr.String(a.stackName),
	}, stackTimeout)
}
