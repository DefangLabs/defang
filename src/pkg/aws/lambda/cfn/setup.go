package cfn

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfnTypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/ptr"
	common "github.com/defang-io/defang/src/pkg/aws"
	"github.com/defang-io/defang/src/pkg/aws/ecs"
	"github.com/defang-io/defang/src/pkg/aws/ecs/cfn/outputs"
	"github.com/defang-io/defang/src/pkg/aws/lambda"
	"github.com/defang-io/defang/src/pkg/aws/region"
	"github.com/defang-io/defang/src/pkg/types"
)

type AwsLambda struct {
	lambda.AwsLambda
	stackName string
}

// var _ types.Driver = (*AwsEcs)(nil)

const stackTimeout = time.Minute * 3

func New(stack string, region region.Region) *AwsLambda {
	if stack == "" {
		panic("stack must be set")
	}
	return &AwsLambda{
		stackName: stack,
		AwsLambda: lambda.AwsLambda{
			Aws: common.Aws{Region: region},
		},
	}
}

func (a *AwsLambda) newClient(ctx context.Context) (*cloudformation.Client, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	return cloudformation.NewFromConfig(cfg), nil
}

func update1s(o *cloudformation.StackUpdateCompleteWaiterOptions) {
	o.MinDelay = 1
}

func (a *AwsLambda) updateStackAndWait(ctx context.Context, templateBody string) error {
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	uso, err := cfn.UpdateStack(ctx, &cloudformation.UpdateStackInput{
		StackName:    ptr.String(a.stackName),
		TemplateBody: ptr.String(templateBody),
		Capabilities: []cfnTypes.Capability{cfnTypes.CapabilityCapabilityNamedIam},
	})
	if err != nil {
		// Go SDK doesn't have --no-fail-on-empty-changeset; ignore ValidationError: No updates are to be performed.
		var apiError smithy.APIError
		if ok := errors.As(err, &apiError); ok && apiError.ErrorCode() == "ValidationError" && apiError.ErrorMessage() == "No updates are to be performed." {
			return a.fillOutputs(ctx)
		}
		// TODO: handle UPDATE_COMPLETE_CLEANUP_IN_PROGRESS
		return err // might call createStackAndWait depending on the error
	}

	fmt.Println("Waiting for stack", a.stackName, "to be updated...") // TODO: verbose only
	o, err := cloudformation.NewStackUpdateCompleteWaiter(cfn, update1s).WaitForOutput(ctx, &cloudformation.DescribeStacksInput{
		StackName: uso.StackId,
	}, stackTimeout)
	if err != nil {
		return err
	}
	return a.fillWithOutputs(ctx, o)
}

func create1s(o *cloudformation.StackCreateCompleteWaiterOptions) {
	o.MinDelay = 1
}

func (a *AwsLambda) createStackAndWait(ctx context.Context, templateBody string) error {
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	_, err = cfn.CreateStack(ctx, &cloudformation.CreateStackInput{
		StackName:    ptr.String(a.stackName),
		TemplateBody: ptr.String(templateBody),
		Capabilities: []cfnTypes.Capability{cfnTypes.CapabilityCapabilityNamedIam},
		OnFailure:    cfnTypes.OnFailureDelete,
	})
	if err != nil {
		// Ignore AlreadyExistsException; return all other errors
		var alreadyExists *cfnTypes.AlreadyExistsException
		if !errors.As(err, &alreadyExists) {
			return err
		}
	}

	fmt.Println("Waiting for stack", a.stackName, "to be created...") // TODO: verbose only
	dso, err := cloudformation.NewStackCreateCompleteWaiter(cfn, create1s).WaitForOutput(ctx, &cloudformation.DescribeStacksInput{
		StackName: ptr.String(a.stackName),
	}, stackTimeout)
	if err != nil {
		return err
	}
	return a.fillWithOutputs(ctx, dso)
}

func (a *AwsLambda) SetUp(ctx context.Context, containers []types.Container) error {
	template, err := createTemplate(a.stackName, containers).YAML()
	if err != nil {
		return err
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

func (a *AwsLambda) fillOutputs(ctx context.Context) error {
	// println("Filling outputs for stack", stackId)
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	// FIXME: this always returns the latest outputs, not the ones from the recent update
	dso, err := cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: &a.stackName,
	})
	if err != nil {
		return err
	}
	return a.fillWithOutputs(ctx, dso)
}

func (a *AwsLambda) fillWithOutputs(ctx context.Context, dso *cloudformation.DescribeStacksOutput) error {
	for _, stack := range dso.Stacks {
		for _, output := range stack.Outputs {
			switch *output.OutputKey {
			case outputs.SubnetID:
				if a.SubNetID == "" {
					a.SubNetID = *output.OutputValue
				}
			case outputs.LogGroupARN:
				a.LogGroupARN = *output.OutputValue
			case outputs.SecurityGroupID:
				a.SecurityGroupID = *output.OutputValue
			case outputs.BucketName:
				a.BucketName = *output.OutputValue
				// default:; TODO: should do this but only for stack the driver created
				// 	return fmt.Errorf("unknown output key %q", *output.OutputKey)
			}
		}
	}

	return nil
}

func (a *AwsLambda) Run(ctx context.Context, env map[string]string, cmd ...string) (ecs.TaskArn, error) {
	if err := a.fillOutputs(ctx); err != nil {
		return nil, err
	}

	return a.AwsLambda.Run(ctx, env, cmd...)
}

func (a *AwsLambda) Tail(ctx context.Context, taskArn ecs.TaskArn) error {
	if err := a.fillOutputs(ctx); err != nil {
		return err
	}
	return a.AwsLambda.Tail(ctx, taskArn)
}

func (a *AwsLambda) Stop(ctx context.Context, taskArn ecs.TaskArn) error {
	if err := a.fillOutputs(ctx); err != nil {
		return err
	}
	return a.AwsLambda.Stop(ctx, taskArn)
}

func (a *AwsLambda) GetInfo(ctx context.Context, taskArn ecs.TaskArn) (string, error) {
	if err := a.fillOutputs(ctx); err != nil {
		return "", err
	}
	return a.AwsLambda.Info(ctx, taskArn)
}

func (a *AwsLambda) TearDown(ctx context.Context) error {
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	_, err = cfn.DeleteStack(ctx, &cloudformation.DeleteStackInput{
		StackName: ptr.String(a.stackName),
		// RetainResources: []string{"Bucket"}, only when the stack is in the DELETE_FAILED state
	})
	if err != nil {
		return err
	}

	fmt.Println("Waiting for stack", a.stackName, "to be deleted...") // TODO: verbose only
	return cloudformation.NewStackDeleteCompleteWaiter(cfn).Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: ptr.String(a.stackName),
	}, stackTimeout)
}
