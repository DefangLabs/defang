package pulumi

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

const taskCount = 1

func (a *AwsEcs) Run(ctx context.Context, env map[string]string, cmd ...string) (TaskArn, error) {
	a.stackOutputs(ctx)

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(string(a.region)))
	if err != nil {
		return nil, err
	}

	// Get the subnet IDs
	subnet, err := ec2.NewFromConfig(cfg).DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{})
	if err != nil {
		return nil, err
	}
	var subnetIds []string
	for _, sn := range subnet.Subnets {
		subnetIds = append(subnetIds, *sn.SubnetId)
	}

	var pairs []types.KeyValuePair
	for k, v := range env {
		pairs = append(pairs, types.KeyValuePair{
			Name:  aws.String(k),
			Value: aws.String(v),
		})
	}

	rti := ecs.RunTaskInput{
		Count:          aws.Int32(taskCount),
		LaunchType:     types.LaunchTypeFargate,
		TaskDefinition: aws.String(a.taskDefArn),
		Cluster:        aws.String(a.clusterArn),
		NetworkConfiguration: &types.NetworkConfiguration{
			AwsvpcConfiguration: &types.AwsVpcConfiguration{
				Subnets: subnetIds, // required
				// 		SecurityGroups: []string{},
				AssignPublicIp: types.AssignPublicIpEnabled,
			},
		},
		Overrides: &types.TaskOverride{
			ContainerOverrides: []types.ContainerOverride{
				{
					Name:        aws.String(containerName),
					Command:     cmd,
					Environment: pairs,
					// EnvironmentFiles: ,
				},
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
