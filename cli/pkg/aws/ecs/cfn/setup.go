package cfn

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfnTypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/defang-io/defang/cli/pkg/aws/ecs"
	"github.com/defang-io/defang/cli/pkg/types"
)

type AwsEcs struct {
	ecs.AwsEcs
	stackName string
}

var _ types.Driver = (*AwsEcs)(nil)

const stackTimeout = time.Minute * 3

func New(stack string, region ecs.Region) *AwsEcs {
	return &AwsEcs{
		stackName: types.StackName(stack),
		AwsEcs: ecs.AwsEcs{
			Region: region,
			Spot:   true,
			VCpu:   1.0,
		},
	}
}

func (a AwsEcs) newClient(ctx context.Context) (*cloudformation.Client, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	return cloudformation.NewFromConfig(cfg), nil
}

func (a *AwsEcs) updateStackAndWait(ctx context.Context, templateBody string) error {
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	uso, err := cfn.UpdateStack(ctx, &cloudformation.UpdateStackInput{
		StackName:    aws.String(a.stackName),
		TemplateBody: aws.String(templateBody),
		Capabilities: []cfnTypes.Capability{cfnTypes.CapabilityCapabilityNamedIam},
	})
	if err != nil {
		// Go SDK doesn't have --no-fail-on-empty-changeset; ignore ValidationError: No updates are to be performed.
		var apiError smithy.APIError
		if ok := errors.As(err, &apiError); ok && apiError.ErrorCode() == "ValidationError" && apiError.ErrorMessage() == "No updates are to be performed." {
			return nil
		}
		return err // might call createStackAndWait depending on the error
	}

	defer a.fillOutputs(ctx, *uso.StackId)

	println("Waiting for stack to be updated...") // TODO: verbose only
	return cloudformation.NewStackUpdateCompleteWaiter(cfn).Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: uso.StackId,
	}, stackTimeout)
}

func (a *AwsEcs) createStackAndWait(ctx context.Context, templateBody string) error {
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	_, err = cfn.CreateStack(ctx, &cloudformation.CreateStackInput{
		StackName:    aws.String(a.stackName),
		TemplateBody: aws.String(templateBody),
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

	println("Waiting for stack to be created...") // TODO: verbose only
	return cloudformation.NewStackCreateCompleteWaiter(cfn).Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(a.stackName),
	}, stackTimeout)
}

func (a *AwsEcs) SetUp(ctx context.Context, image string, memory uint64) error {
	template, err := createTemplate(image, float64(memory)/1024/1024, a.VCpu, a.Spot).YAML()
	if err != nil {
		return err
	}

	// Upsert
	if err := a.updateStackAndWait(ctx, string(template)); err != nil {
		// Check if the stack doesn't exist; if so, create it, otherwise return the error
		var apiError smithy.APIError
		if ok := errors.As(err, &apiError); !ok || apiError.ErrorCode() != "ValidationError" || !strings.HasSuffix(apiError.ErrorMessage(), "does not exist") {
			return err
		}

		return a.createStackAndWait(ctx, string(template))
	}
	return nil
}

func (a *AwsEcs) fillOutputs(ctx context.Context, stackId string) error {
	// println("Filling outputs for stack", stackId)
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	// FIXME: this always returns the latest outputs, not the ones from the recent update
	dso, err := cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackId,
	})
	if err != nil {
		return err
	}
	for _, stack := range dso.Stacks {
		for _, output := range stack.Outputs {
			switch *output.OutputKey {
			case "taskDefArn":
				if a.TaskDefArn == "" {
					a.TaskDefArn = *output.OutputValue
				}
			case "clusterArn":
				a.ClusterArn = *output.OutputValue
			case "logGroupName":
				a.LogGroupName = *output.OutputValue
			}
		}
	}

	return nil
}

func (a *AwsEcs) Run(ctx context.Context, env map[string]string, cmd ...string) (ecs.TaskArn, error) {
	if err := a.fillOutputs(ctx, a.stackName); err != nil {
		return nil, err
	}

	return a.AwsEcs.Run(ctx, env, cmd...)
}

func (a *AwsEcs) Tail(ctx context.Context, taskArn ecs.TaskArn) error {
	if err := a.fillOutputs(ctx, a.stackName); err != nil {
		return err
	}
	return a.AwsEcs.Tail(ctx, taskArn)
}

func (a *AwsEcs) TearDown(ctx context.Context) error {
	cfn, err := a.newClient(ctx)
	if err != nil {
		return err
	}

	_, err = cfn.DeleteStack(ctx, &cloudformation.DeleteStackInput{
		StackName: aws.String(a.stackName),
		// RetainResources: []string{"Bucket"}, only when the stack is in the DELETE_FAILED state
	})
	if err != nil {
		return err
	}

	println("Waiting for stack to be deleted...") // TODO: verbose only
	return cloudformation.NewStackDeleteCompleteWaiter(cfn).Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(a.stackName),
	}, stackTimeout)
}
