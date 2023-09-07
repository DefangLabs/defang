package cfn

import (
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/awslabs/goformation/v7/cloudformation"
	"github.com/awslabs/goformation/v7/cloudformation/ecr"
	"github.com/awslabs/goformation/v7/cloudformation/ecs"
	"github.com/awslabs/goformation/v7/cloudformation/iam"
	"github.com/awslabs/goformation/v7/cloudformation/logs"
	"github.com/awslabs/goformation/v7/cloudformation/s3"
	awsecs "github.com/defang-io/defang/cli/pkg/aws/ecs"
)

func createTemplate(image string, memory float64, vcpu float64, spot bool) *cloudformation.Template {
	// const PREFIX = "my-"
	// const SUFFIX = ""
	const ecrPublicPrefix = "ecr-public3" // TODO: change; use project/stack name

	template := cloudformation.NewTemplate()

	// 1. bucket (for state)
	template.Resources["Bucket"] = &s3.Bucket{
		// BucketName: aws.String(PREFIX + "bucket" + SUFFIX), // optional
	}

	// 2. ECS cluster
	template.Resources["Cluster"] = &ecs.Cluster{
		// ClusterName: aws.String(PREFIX + "cluster" + SUFFIX), // optional
	}

	// 3. ECR pull-through cache
	template.Resources["PullThroughCache"] = &ecr.PullThroughCacheRule{
		EcrRepositoryPrefix: aws.String(ecrPublicPrefix), // TODO: optional
		UpstreamRegistryUrl: aws.String(awsecs.EcrPublicRegistry),
	}

	// 4. ECS capacity provider
	capacityProvider := "FARGATE"
	if spot {
		capacityProvider = "FARGATE_SPOT"
	}
	template.Resources["CapacityProvider"] = &ecs.ClusterCapacityProviderAssociations{
		Cluster: cloudformation.Ref("Cluster"),
		CapacityProviders: []string{
			capacityProvider,
		},
		DefaultCapacityProviderStrategy: []ecs.ClusterCapacityProviderAssociations_CapacityProviderStrategy{
			{
				CapacityProvider: capacityProvider,
				Weight:           aws.Int(1),
			},
		},
	}

	// 5. CloudWatch log group
	template.Resources["LogGroup"] = &logs.LogGroup{
		// LogGroupName:    aws.String(PREFIX + "log-group-test" + SUFFIX), // optional
		RetentionInDays: aws.Int(1),
	}

	// 6. IAM role for task
	template.Resources["TaskRole"] = &iam.Role{
		// RoleName: aws.String(PREFIX + "task-role" + SUFFIX), // optional
		ManagedPolicyArns: []string{
			"arn:aws:iam::aws:policy/PowerUserAccess",
		},
		AssumeRolePolicyDocument: map[string]any{
			"Version": "2012-10-17",
			"Statement": []map[string]any{
				{
					"Effect": "Allow",
					"Principal": map[string]any{
						"Service": []string{
							"ecs-tasks.amazonaws.com",
						},
					},
					"Action": []string{
						"sts:AssumeRole",
					},
				},
			},
		},
	}

	// 7. ECS task definition
	if repo, ok := strings.CutPrefix(image, awsecs.EcrPublicRegistry); ok {
		image = cloudformation.Sub("${AWS::AccountId}.dkr.ecr.${AWS::Region}.amazonaws.com/" + ecrPublicPrefix + repo)
	}
	cpu, mem := awsecs.FixupFargateConfig(vcpu, memory)
	template.Resources["TaskDefinition"] = &ecs.TaskDefinition{
		ContainerDefinitions: []ecs.TaskDefinition_ContainerDefinition{
			{
				Name:  awsecs.ContainerName,
				Image: image,
				LogConfiguration: &ecs.TaskDefinition_LogConfiguration{
					LogDriver: "awslogs",
					Options: map[string]string{
						"awslogs-group":         cloudformation.Ref("LogGroup"),
						"awslogs-region":        cloudformation.Ref("AWS::Region"),
						"awslogs-stream-prefix": awsecs.StreamPrefix,
					},
				},
				Environment: []ecs.TaskDefinition_KeyValuePair{
					{
						Name:  aws.String("PULUMI_BACKEND_URL"),
						Value: cloudformation.SubPtr("s3://${Bucket}?region=${AWS::Region}&awssdk=v2"),
					},
					{
						Name:  aws.String("PULUMI_SKIP_UPDATE_CHECK"),
						Value: aws.String("true"),
					},
					{
						Name:  aws.String("PULUMI_SKIP_CONFIRMATIONS"),
						Value: aws.String("true"),
					},
				},
			},
		},
		Cpu:                     aws.String(strconv.FormatUint(uint64(cpu), 10)),
		ExecutionRoleArn:        cloudformation.RefPtr("TaskRole"),
		Memory:                  aws.String(strconv.FormatUint(uint64(mem), 10)),
		NetworkMode:             aws.String("awsvpc"),
		RequiresCompatibilities: []string{"FARGATE"},
		TaskRoleArn:             cloudformation.RefPtr("TaskRole"),
		// Family:                  aws.String(PREFIX + "task-def" + SUFFIX), // optional
	}

	template.Outputs["taskDefArn"] = cloudformation.Output{
		Value:       cloudformation.Ref("TaskDefinition"),
		Description: aws.String("ARN of the ECS task definition"),
	}

	template.Outputs["clusterArn"] = cloudformation.Output{
		Value:       cloudformation.Ref("Cluster"),
		Description: aws.String("ARN of the ECS cluster"),
	}

	template.Outputs["logGroupName"] = cloudformation.Output{
		Value:       cloudformation.Ref("LogGroup"),
		Description: aws.String("Name of the CloudWatch log group"),
	}

	return template
}
