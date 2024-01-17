package ecs

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/smithy-go/ptr"
)

const taskCount = 1

func (a *AwsEcs) Run(ctx context.Context, env map[string]string, cmd ...string) (TaskArn, error) {
	// a.Refresh(ctx)

	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	if a.SubNetID == "" {
		// Get a subnet ID
		subnetsOutput, err := ec2.NewFromConfig(cfg).DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
			Filters: []ec2types.Filter{
				{
					Name:   ptr.String("vpc-id"),
					Values: []string{a.VpcID},
					// 		Name:   ptr.String("map-public-ip-on-launch"),
					// 		Values: []string{"true"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		a.SubNetID = *subnetsOutput.Subnets[0].SubnetId // TODO: make configurable/deterministic
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

	var securityGroups []string
	if a.SubNetID == "" {
		securityGroups = []string{a.SecurityGroupID} // TODO: only if ports are mapped
	}
	rti := ecs.RunTaskInput{
		Count:          ptr.Int32(taskCount),
		LaunchType:     types.LaunchTypeFargate,
		TaskDefinition: ptr.String(a.TaskDefARN),
		PropagateTags:  types.PropagateTagsTaskDefinition,
		Cluster:        ptr.String(a.ClusterName),
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
					Name:        ptr.String(ContainerName),
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
				Value: ptr.String(time.Now().String()),
			},
		},
	}

	ecsOutput, err := ecs.NewFromConfig(cfg).RunTask(ctx, &rti)
	if err != nil {
		return nil, err
	}
	failures := make([]error, len(ecsOutput.Failures))
	for i, f := range ecsOutput.Failures {
		failures[i] = taskFailure{*f.Reason, *f.Detail}
	}
	if err := errors.Join(failures...); err != nil || len(ecsOutput.Tasks) == 0 {
		return nil, err
	}
	// bytes, _ := json.MarshalIndent(ecsOutput.Tasks, "", "  ")
	// println(string(bytes))
	return TaskArn(ecsOutput.Tasks[0].TaskArn), nil
}

type taskFailure struct {
	Reason string
	Detail string
}

func (t taskFailure) Error() string {
	return t.Reason + ": " + t.Detail
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
