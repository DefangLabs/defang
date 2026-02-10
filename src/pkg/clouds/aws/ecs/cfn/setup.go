package cfn

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	awsecs "github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfnTypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/ptr"
)

type AwsEcsCfn struct {
	awsecs.AwsEcs
	stackName string
}

const stackTimeout = time.Minute * 3

func OptionVPCAndSubnetID(ctx context.Context, vpcID, subnetID string) func(clouds.Driver) error {
	return func(d clouds.Driver) error {
		if ecs, ok := d.(*AwsEcsCfn); ok {
			return ecs.PopulateVPCandSubnetID(ctx, vpcID, subnetID)
		}
		return errors.New("only AwsEcs driver supports VPC ID and Subnet ID option")
	}
}

func New(stack string, region aws.Region) *AwsEcsCfn {
	if stack == "" {
		panic("stack must be set")
	}
	return &AwsEcsCfn{
		stackName: stack,
		AwsEcs: awsecs.AwsEcs{
			Aws:          aws.Aws{Region: region},
			CDRegion:     region, // assume CD runs in the same region as the app
			RetainBucket: true,
			// Spot: true,
		},
	}
}

func (a *AwsEcsCfn) newCloudFormationClient(ctx context.Context) (*cloudformation.Client, error) {
	cfg, err := a.LoadConfigForCD(ctx)
	if err != nil {
		return nil, err
	}
	return cloudformation.NewFromConfig(cfg), nil
}

func withRegion(region aws.Region) func(*cloudformation.Options) {
	return func(opts *cloudformation.Options) {
		if region != "" {
			opts.Region = string(region)
		}
	}
}

func (a *AwsEcsCfn) describeStacksAllRegions(ctx context.Context, cfn *cloudformation.Client) (*cloudformation.DescribeStacksOutput, error) {
	input := &cloudformation.DescribeStacksInput{StackName: &a.stackName}

	// First, try the current region of the CloudFormation client
	var noStackErr error
	if dso, err := cfn.DescribeStacks(ctx, input); err != nil {
		err = annotateCfnError(err)
		if snf := new(ErrStackNotFound); !errors.As(err, &snf) {
			return nil, err
		}
		// CD stack not found in this region; try other regions before returning not found error
		noStackErr = err
	} else {
		return dso, nil
	}

	// Use a single S3 query to list all buckets with the defang-cd- prefix
	// This is faster than calling CloudFormation DescribeStacks in each region
	cfg, err := a.LoadConfigForCD(ctx)
	if err != nil {
		return nil, err
	}
	buckets, err := aws.ListBucketsByPrefix(ctx, s3.NewFromConfig(cfg), a.stackName+"-")
	if err != nil {
		return nil, err
	}
	for _, bucketRegion := range buckets {
		if bucketRegion == a.CDRegion {
			continue // skip, already done above
		}
		if dso, err := cfn.DescribeStacks(ctx, input, withRegion(bucketRegion)); err == nil {
			a.CDRegion = bucketRegion
			term.Debug("Reusing CloudFormation stack in region", bucketRegion)
			return dso, nil
		}
	}
	return nil, noStackErr
}

func (a *AwsEcsCfn) updateStackAndWait(ctx context.Context, templateBody string, parameters []cfnTypes.Parameter) error {
	cfn, err := a.newCloudFormationClient(ctx)
	if err != nil {
		return err
	}

	// Check the template version first, to avoid updating to an outdated template; TODO: can we use StackPolicy/Conditions instead?
	if dso, err := a.describeStacksAllRegions(ctx, cfn); err == nil && len(dso.Stacks) == 1 {
		for _, output := range dso.Stacks[0].Outputs {
			if *output.OutputKey == OutputsTemplateVersion {
				deployedRev, _ := strconv.Atoi(*output.OutputValue)
				if deployedRev > TemplateRevision {
					return fmt.Errorf("This CLI has an older CloudFormation template than the deployed %s stack: please update the CLI", a.stackName)
				}
			}
		}
		// Set "Use previous value" for parameters not in the new parameters list
		newParams := map[string]struct{}{}
		for _, newParam := range parameters {
			newParams[*newParam.ParameterKey] = struct{}{}
		}
		for _, param := range dso.Stacks[0].Parameters {
			if _, ok := newParams[*param.ParameterKey]; !ok {
				parameters = append(parameters, cfnTypes.Parameter{
					ParameterKey:     param.ParameterKey,
					UsePreviousValue: ptr.Bool(true),
				})
			}
		}
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

	term.Infof("Waiting for CloudFormation stack %s to be updated in %s...", a.stackName, a.CDRegion)
	dso, err := cloudformation.NewStackUpdateCompleteWaiter(cfn, update1s).WaitForOutput(ctx, &cloudformation.DescribeStacksInput{
		StackName: uso.StackId,
	}, stackTimeout)
	if err != nil {
		return fmt.Errorf("failed to update CloudFormation stack: check the CloudFormation console (https://%s.console.aws.amazon.com/cloudformation/home) for the %q stack to learn more: %w", a.AwsEcs.Region, a.stackName, err)
	}
	return a.fillWithOutputs(dso)
}

func (a *AwsEcsCfn) createStackAndWait(ctx context.Context, templateBody string, parameters []cfnTypes.Parameter) error {
	cfn, err := a.newCloudFormationClient(ctx)
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

	term.Infof("Waiting for CloudFormation stack %s to be created in %s...", a.stackName, a.CDRegion)
	dso, err := cloudformation.NewStackCreateCompleteWaiter(cfn, create1s).WaitForOutput(ctx, &cloudformation.DescribeStacksInput{
		StackName: ptr.String(a.stackName),
	}, stackTimeout)
	if err != nil {
		return fmt.Errorf("failed to create CloudFormation stack: check the CloudFormation console (https://%s.console.aws.amazon.com/cloudformation/home) for the %q stack to learn more: %w", a.AwsEcs.Region, a.stackName, err)
	}
	return a.fillWithOutputs(dso)
}

func (a *AwsEcsCfn) SetUp(ctx context.Context, containers []clouds.Container) error {
	template, err := CreateTemplate(a.stackName, containers)
	if err != nil {
		return fmt.Errorf("failed to create CloudFormation template: %w", err)
	}

	templateBody, err := template.YAML()
	if err != nil {
		return err
	}

	// Set parameter values based on current configuration
	parameters := []cfnTypes.Parameter{
		// {
		// 	ParameterKey:   ptr.String(ParamsCIRoleName),
		// 	ParameterValue: ptr.String("defang-cd-CDIRole-us-west-2"),
		// },
		{
			ParameterKey:   ptr.String(ParamsExistingVpcId),
			ParameterValue: ptr.String(a.VpcID),
		},
		{
			ParameterKey:   ptr.String(ParamsRetainBucket),
			ParameterValue: ptr.String(strconv.FormatBool(a.RetainBucket)),
		},
		{
			ParameterKey:   ptr.String(ParamsEnablePullThroughCache),
			ParameterValue: ptr.String(strconv.FormatBool(!pkg.GetenvBool("DEFANG_NO_CACHE"))),
		},
	}

	// Add Docker Hub credentials if available from environment
	if dockerHubUsername := os.Getenv("DOCKERHUB_USERNAME"); dockerHubUsername != "" {
		parameters = append(parameters, cfnTypes.Parameter{
			ParameterKey:   ptr.String(ParamsDockerHubUsername),
			ParameterValue: ptr.String(dockerHubUsername),
		})
	}
	if dockerHubToken := os.Getenv("DOCKERHUB_ACCESS_TOKEN"); dockerHubToken != "" {
		parameters = append(parameters, cfnTypes.Parameter{
			ParameterKey:   ptr.String(ParamsDockerHubAccessToken),
			ParameterValue: ptr.String(dockerHubToken),
		})
	}
	// TODO: support DOCKER_AUTH_CONFIG

	return a.upsertStackAndWait(ctx, templateBody, parameters...)
}

func (a *AwsEcsCfn) upsertStackAndWait(ctx context.Context, templateBody []byte, parameters ...cfnTypes.Parameter) error {
	// Upsert with parameters
	if err := a.updateStackAndWait(ctx, string(templateBody), parameters); err != nil {
		// Check if the stack doesn't exist; if so, create it, otherwise return the error
		err = annotateCfnError(err)
		if snf := new(ErrStackNotFound); !errors.As(err, &snf) {
			return err
		}
		return a.createStackAndWait(ctx, string(templateBody), parameters)
	}
	return nil
}

type ErrStackNotFound = cfnTypes.StackNotFoundException

func annotateCfnError(err error) error {
	// Check if the stack doesn't exist (ValidationError); if so, return a nice error; workaround for https://github.com/aws/aws-sdk-go-v2/issues/2296
	var ae smithy.APIError
	if errors.As(err, &ae) && ae.ErrorCode() == "ValidationError" && strings.HasSuffix(ae.ErrorMessage(), " does not exist") {
		err = &ErrStackNotFound{Message: ptr.String(ae.ErrorMessage())}
	}
	return err
}

func (a *AwsEcsCfn) FillOutputs(ctx context.Context) error {
	cfn, err := a.newCloudFormationClient(ctx)
	if err != nil {
		return err
	}

	// NOTE: this always returns the latest outputs, not the ones from the recent update
	dso, err := cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: ptr.String(a.stackName),
	})
	if err != nil {
		return annotateCfnError(err)
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
		case OutputsTaskDefARN:
			a.TaskDefARN = *output.OutputValue
		case OutputsClusterName:
			a.ClusterName = *output.OutputValue
		case OutputsLogGroupARN:
			a.LogGroupARN = *output.OutputValue
		case OutputsSecurityGroupID:
			a.SecurityGroupID = *output.OutputValue
		case OutputsBucketName:
			a.BucketName = *output.OutputValue
		case OutputsCIRoleARN:
			a.CIRoleARN = *output.OutputValue
		}
	}

	if a.AccountID == "" && a.LogGroupARN != "" {
		a.AccountID = aws.GetAccountID(a.LogGroupARN)
	}
	return nil
}

func (a *AwsEcsCfn) Run(ctx context.Context, env map[string]string, cmd ...string) (awsecs.TaskArn, error) {
	if err := a.FillOutputs(ctx); err != nil {
		return nil, err
	}

	return a.AwsEcs.Run(ctx, env, cmd...)
}

func (a *AwsEcsCfn) Tail(ctx context.Context, taskArn awsecs.TaskArn) error {
	if err := a.FillOutputs(ctx); err != nil {
		return err
	}
	return a.AwsEcs.Tail(ctx, taskArn)
}

func (a *AwsEcsCfn) Stop(ctx context.Context, taskArn awsecs.TaskArn) error {
	if err := a.FillOutputs(ctx); err != nil {
		return err
	}
	return a.AwsEcs.Stop(ctx, taskArn)
}

func (a *AwsEcsCfn) GetInfo(ctx context.Context, taskArn awsecs.TaskArn) (*clouds.TaskInfo, error) {
	if err := a.FillOutputs(ctx); err != nil {
		return nil, err
	}
	return a.AwsEcs.Info(ctx, taskArn)
}

func (a *AwsEcsCfn) TearDown(ctx context.Context) error {
	cfn, err := a.newCloudFormationClient(ctx)
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

	term.Infof("Waiting for CloudFormation stack %s to be deleted in %s...", a.stackName, a.CDRegion)
	return cloudformation.NewStackDeleteCompleteWaiter(cfn, delete1s).Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: ptr.String(a.stackName),
	}, stackTimeout)
}
