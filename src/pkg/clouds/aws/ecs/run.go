package ecs

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/smithy-go/ptr"
)

const taskCount = 1

func (a *AwsEcs) PopulateVPCandSubnetID(ctx context.Context, vpcID, subnetID string) error {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	if vpcID != "" && subnetID == "" {
		subnetID, err = getPublicSubnetId(ctx, cfg, vpcID)
	} else if vpcID == "" && subnetID != "" {
		vpcID, err = getSubnetVPCId(ctx, cfg, subnetID)
	}

	a.VpcID = vpcID
	a.SubNetID = subnetID
	return err
}

var sanitizeStartedBy = regexp.MustCompile(`[^a-zA-Z0-9_-]+`) // letters (uppercase and lowercase), numbers, hyphens (-), and underscores (_) are allowed

func (a *AwsEcs) Run(ctx context.Context, env map[string]string, cmd ...string) (TaskArn, error) {
	// a.Refresh(ctx)

	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	var pairs []types.KeyValuePair
	for k, v := range env {
		pairs = append(pairs, types.KeyValuePair{
			Name:  ptr.String(k),
			Value: ptr.String(v),
		})
	}

	// stsClient := sts.NewFromConfig(cfg)
	// cred, err := stsClient.GetCallerIdentity(ctx, nil)
	// if err != nil {
	// 	return nil, err
	// }

	securityGroups := []string{a.SecurityGroupID} // TODO: only if ports are mapped
	rti := ecs.RunTaskInput{
		Count:          ptr.Int32(taskCount),
		LaunchType:     types.LaunchTypeFargate,
		TaskDefinition: ptr.String(a.TaskDefARN),
		PropagateTags:  types.PropagateTagsTaskDefinition,
		Cluster:        ptr.String(a.ClusterName),
		StartedBy:      ptr.String(sanitizeStartedBy.ReplaceAllLiteralString(pkg.GetCurrentUser(), "_")),
		NetworkConfiguration: &types.NetworkConfiguration{
			AwsvpcConfiguration: &types.AwsVpcConfiguration{
				AssignPublicIp: types.AssignPublicIpEnabled, // only works with public subnets
				Subnets:        []string{a.SubNetID},        // TODO: make configurable; must this match the VPC of the SecGroup?
				SecurityGroups: securityGroups,
			},
		},
		Overrides: &types.TaskOverride{
			// Cpu:   ptr.String("256"),
			// Memory: ptr.String("512"),
			// TaskRoleArn: cred.Arn; TODO: default to caller identity; needs trust + iam:PassRole
			ContainerOverrides: []types.ContainerOverride{
				{
					Name:        ptr.String(CdContainerName),
					Command:     cmd,
					Environment: pairs,
					// ResourceRequirements:; TODO: make configurable, support GPUs
					// EnvironmentFiles: ,
				},
			},
		},
		Tags: []types.Tag{ //TODO: add tags to the task
			{
				Key:   ptr.String("StartedAt"),
				Value: ptr.String(time.Now().Format(time.RFC3339)),
			},
			{
				Key:   ptr.String("StartedBy"),
				Value: ptr.String(pkg.GetCurrentUser()),
			},
		},
	}

	ecsOutput, err := ecs.NewFromConfig(cfg).RunTask(ctx, &rti)
	if err != nil {
		return nil, err
	}
	failures := make([]error, len(ecsOutput.Failures))
	for i, f := range ecsOutput.Failures {
		failures[i] = TaskFailure{types.TaskStopCode(*f.Reason), *f.Detail}
	}
	if err := errors.Join(failures...); err != nil {
		return nil, err
	}
	if len(ecsOutput.Tasks) == 0 || ecsOutput.Tasks[0].TaskArn == nil {
		return nil, errors.New("no task started")
	}
	// bytes, _ := json.MarshalIndent(ecsOutput.Tasks, "", "  ")
	// println(string(bytes))
	return TaskArn(ecsOutput.Tasks[0].TaskArn), nil
}

func getPublicSubnetId(ctx context.Context, cfg aws.Config, vpcId string) (string, error) {
	subnetsOutput, err := ec2.NewFromConfig(cfg).DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []ec2types.Filter{
			{
				Name:   ptr.String("vpc-id"),
				Values: []string{vpcId},
			},
			{
				Name:   ptr.String("map-public-ip-on-launch"),
				Values: []string{"true"},
			},
		},
	})
	if err != nil {
		return "", err
	}
	return *subnetsOutput.Subnets[0].SubnetId, nil // TODO: make configurable/deterministic
}

func getSubnetVPCId(ctx context.Context, cfg aws.Config, subnetId string) (string, error) {
	subnetsOutput, err := ec2.NewFromConfig(cfg).DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		SubnetIds: []string{subnetId},
	})
	if err != nil {
		return "", err
	}
	return *subnetsOutput.Subnets[0].VpcId, nil // TODO: make configurable/deterministic
}

type TaskFailure struct {
	Reason types.TaskStopCode
	Detail string
}

func (t TaskFailure) Error() string {
	return string(t.Reason) + ": " + t.Detail
}

/*
func getAwsEnv() awsEnv {
	creds := getEcsCreds()
	return map[string]string{
		"AWS_ACCESS_KEY_ID":     creds.AccessKeyId,
		"AWS_SECRET_ACCESS_KEY": creds.SecretAccessKey,
		"AWS_SESSION_TOKEN":     creds.Token,
		// "AWS_REGION": "us-west-2", should not be needed because it's in the stack config and/or env
	}
}

var (
	ecsCredsUrl = "http://169.254.170.2" + os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI")
)

type ecsCreds struct {
	AccessKeyId     string
	Expiration      string
	RoleArn         string
	SecretAccessKey string
	Token           string
}

func getEcsCreds() (creds ecsCreds) {
	// Grab the ECS credentials from the metadata service at AWS_CONTAINER_CREDENTIALS_RELATIVE_URI
	res, err := http.Get(ecsCredsUrl)
	if err != nil {
		log.Panicln(err)
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(&creds); err != nil {
		log.Panicln(err)
	}
	return creds
}
*/
