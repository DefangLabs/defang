package cfn

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	awscodebuild "github.com/DefangLabs/defang/src/pkg/clouds/aws/codebuild"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfnTypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/ptr"
)

type AwsCfn struct {
	awscodebuild.AwsCodeBuild
	stackName string
	fillOnce  sync.Once
	fillErr   error
}

const stackTimeout = time.Minute * 3

func New(stack string, region aws.Region) *AwsCfn {
	if stack == "" {
		panic("stack must be set")
	}
	return &AwsCfn{
		stackName: stack,
		AwsCodeBuild: awscodebuild.AwsCodeBuild{
			Aws:          aws.Aws{Region: region},
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

	// A new stack has no previous parameter values; drop UsePreviousValue params so the template defaults apply.
	parameters = slices.DeleteFunc(slices.Clone(parameters), func(p cfnTypes.Parameter) bool {
		return p.UsePreviousValue != nil && *p.UsePreviousValue
	})

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

// bucketLister is the subset of the S3 API used to discover an existing CD state bucket.
type bucketLister interface {
	ListBuckets(context.Context, *s3.ListBucketsInput, ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
	GetBucketLocation(context.Context, *s3.GetBucketLocationInput, ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error)
	GetBucketTagging(context.Context, *s3.GetBucketTaggingInput, ...func(*s3.Options)) (*s3.GetBucketTaggingOutput, error)
}

// findAdoptableBucket returns the name of a previously-created CD state bucket in
// this account+region (identified by the `defang-cd-bucket-` prefix and the
// stack-prefix tag), or "" if none exists. It errors if more than one candidate
// is found, since adoption would be ambiguous. Adopting a retained bucket instead
// of creating a new one is what keeps teardown/re-bootstrap from orphaning buckets.
func findAdoptableBucket(ctx context.Context, client bucketLister, stackName string, region aws.Region) (string, error) {
	lbo, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return "", err
	}
	prefix := stackName + "-bucket-"
	var found []string
	for _, b := range lbo.Buckets {
		if b.Name == nil || !strings.HasPrefix(*b.Name, prefix) {
			continue
		}
		// Only adopt a bucket in the target region (CD state is per-region).
		loc, err := client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: b.Name})
		if err != nil {
			continue
		}
		bucketRegion := aws.Region(loc.LocationConstraint)
		if bucketRegion == "" {
			bucketRegion = "us-east-1" // S3 returns an empty LocationConstraint for us-east-1
		}
		if bucketRegion != region {
			continue
		}
		// Confirm it is one of ours via the stack-prefix tag.
		gto, err := client.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: b.Name})
		if err != nil {
			continue // no tags / access denied: treat as not ours
		}
		for _, t := range gto.TagSet {
			if t.Key != nil && *t.Key == TagKeyPrefix && t.Value != nil && *t.Value == stackName {
				found = append(found, *b.Name)
				break
			}
		}
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("found multiple %s state buckets in %s; cannot adopt automatically, remove the stale one(s): %v", stackName, region, found)
	}
}

func (a *AwsCfn) findExistingBucket(ctx context.Context) (string, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return "", err
	}
	return findAdoptableBucket(ctx, s3.NewFromConfig(cfg), a.stackName, a.AwsCodeBuild.Region)
}

func (a *AwsCfn) SetUp(ctx context.Context, force bool) (bool, error) {
	// Reuse a retained CD bucket from a previous stack if one exists, so
	// teardown + re-bootstrap doesn't orphan buckets (see #358, #2192).
	existingBucket, err := a.findExistingBucket(ctx)
	if err != nil {
		return false, err
	}

	template, err := CreateTemplate(a.stackName, existingBucket)
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
	a.fillOnce.Do(func() {
		a.fillErr = a.describeAndFillOutputs(ctx)
	})
	return a.fillErr
}

func (a *AwsCfn) describeAndFillOutputs(ctx context.Context) error {
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
		a.AccountID = aws.GetAccountID(a.LogGroupARN)
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
