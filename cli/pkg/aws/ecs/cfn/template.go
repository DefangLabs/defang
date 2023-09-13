package cfn

import (
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/awslabs/goformation/v7/cloudformation"
	"github.com/awslabs/goformation/v7/cloudformation/ec2"
	"github.com/awslabs/goformation/v7/cloudformation/ecr"
	"github.com/awslabs/goformation/v7/cloudformation/ecs"
	"github.com/awslabs/goformation/v7/cloudformation/iam"
	"github.com/awslabs/goformation/v7/cloudformation/logs"
	"github.com/awslabs/goformation/v7/cloudformation/s3"
	"github.com/awslabs/goformation/v7/cloudformation/tags"
	awsecs "github.com/defang-io/defang/cli/pkg/aws/ecs"
)

func createTemplate(image string, memory float64, vcpu float64, spot bool) *cloudformation.Template {
	const prefix = awsecs.ProjectName + "-" // TODO: include stack name
	const ecrPublicPrefix = prefix + "ecr-public"

	defaultTags := []tags.Tag{
		{
			Key:   "CreatedBy",
			Value: awsecs.ProjectName,
		},
	}

	template := cloudformation.NewTemplate()

	// 1. bucket (for state)
	template.Resources["Bucket"] = &s3.Bucket{
		Tags: defaultTags,
		// BucketName: aws.String(PREFIX + "bucket" + SUFFIX), // optional; TODO: might want to fix this name to allow Pulumi destroy after stack deletion
		AWSCloudFormationDeletionPolicy: "RetainExceptOnCreate",
	}

	// 2. ECS cluster
	template.Resources["Cluster"] = &ecs.Cluster{
		Tags: defaultTags,
		// ClusterName: aws.String(PREFIX + "cluster" + SUFFIX), // optional
	}

	// 3. ECR pull-through cache (only really needed for images from ECR Public registry)
	// TODO: Creating pull through cache rules isn't supported in the following Regions:
	// * China (Beijing) (cn-north-1)
	// * China (Ningxia) (cn-northwest-1)
	// * AWS GovCloud (US-East) (us-gov-east-1)
	// * AWS GovCloud (US-West) (us-gov-west-1)
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
		Tags: defaultTags,
		// LogGroupName:    aws.String(PREFIX + "log-group-test" + SUFFIX), // optional
		RetentionInDays: aws.Int(1),
	}

	// 6. IAM exec role for task
	assumeRolePolicyDocumentECS := map[string]any{
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
	}
	template.Resources["ExecutionRole"] = &iam.Role{
		Tags: defaultTags,
		// RoleName: aws.String(PREFIX + "execution-role" + SUFFIX), // optional
		ManagedPolicyArns: []string{
			"arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy",
		},
		AssumeRolePolicyDocument: assumeRolePolicyDocumentECS,
		Policies: []iam.Role_Policy{
			// {
			// 	PolicyName: "AllowGetSecrets",
			// 	PolicyDocument: map[string]any{
			// 		"Version": "2012-10-17",
			// 		"Statement": []map[string]any{
			// 			{
			// 				"Effect": "Allow",
			// 				"Action": []string{
			// 					"secretsmanager:GetSecretValue",
			// 					"ssm:GetParameters",
			// 					// "kms:Decrypt", Required only if your key uses a custom KMS key and not the default key
			// 				},
			// 				"Resource": "*", // TODO: restrict to ECR or other repo secrets
			// 			},
			// 		},
			// 	},
			// },
			{
				// From https://docs.aws.amazon.com/AmazonECR/latest/userguide/pull-through-cache.html#pull-through-cache-iam
				PolicyName: "AllowECRPassThrough",
				PolicyDocument: map[string]any{
					"Version": "2012-10-17",
					"Statement": []map[string]any{
						{
							"Effect": "Allow",
							"Action": []string{
								"ecr:CreatePullThroughCacheRule",
								"ecr:BatchImportUpstreamImage", // can bse repo permissions instead
								"ecr:CreateRepository",         // can be registry permissions instead
							},
							"Resource": "*", // TODO: restrict
						},
					},
				},
			},
		},
	}

	// 6b. IAM role for task (optional)
	template.Resources["TaskRole"] = &iam.Role{
		Tags: defaultTags,
		// RoleName: aws.String(PREFIX + "task-role" + SUFFIX), // optional
		ManagedPolicyArns: []string{
			"arn:aws:iam::aws:policy/AdministratorAccess", // TODO: make this configurable
		},
		AssumeRolePolicyDocument: assumeRolePolicyDocumentECS,
	}

	// 7. ECS task definition
	if repo, ok := strings.CutPrefix(image, awsecs.EcrPublicRegistry); ok {
		image = cloudformation.Sub("${AWS::AccountId}.dkr.ecr.${AWS::Region}.amazonaws.com/" + ecrPublicPrefix + repo)
	}
	cpu, mem := awsecs.FixupFargateConfig(vcpu, memory)
	template.Resources["TaskDefinition"] = &ecs.TaskDefinition{
		Tags: defaultTags,
		ContainerDefinitions: []ecs.TaskDefinition_ContainerDefinition{
			{
				Name:  awsecs.ContainerName,
				Image: image,
				LogConfiguration: &ecs.TaskDefinition_LogConfiguration{
					LogDriver: "awslogs",
					Options: map[string]string{
						"awslogs-group":         cloudformation.Ref("LogGroup"),
						"awslogs-region":        cloudformation.Ref("AWS::Region"),
						"awslogs-stream-prefix": awsecs.ProjectName,
					},
				},
				Environment: []ecs.TaskDefinition_KeyValuePair{
					{
						Name:  aws.String("PULUMI_BACKEND_URL"), // TODO: this should not be here
						Value: cloudformation.SubPtr("s3://${Bucket}?region=${AWS::Region}&awssdk=v2"),
					},
				},
			},
		},
		Cpu:                     aws.String(strconv.FormatUint(uint64(cpu), 10)),
		ExecutionRoleArn:        cloudformation.RefPtr("ExecutionRole"),
		Memory:                  aws.String(strconv.FormatUint(uint64(mem), 10)),
		NetworkMode:             aws.String("awsvpc"),
		RequiresCompatibilities: []string{"FARGATE"},
		TaskRoleArn:             cloudformation.RefPtr("TaskRole"),
		// Family:                  cloudformation.SubPtr("${AWS::StackName}-TaskDefinition"), // optional, but needed to avoid TaskDef replacement
	}

	// 8. a VPC
	template.Resources["VPC"] = &ec2.VPC{
		Tags:      defaultTags, // TODO: add Name tag
		CidrBlock: aws.String("10.0.0.0/16"),
	}
	// 8b. an internet gateway
	template.Resources["InternetGateway"] = &ec2.InternetGateway{
		Tags: defaultTags,
	}
	template.Resources["InternetGatewayAttachment"] = &ec2.VPCGatewayAttachment{
		VpcId:             cloudformation.Ref("VPC"),
		InternetGatewayId: cloudformation.RefPtr("InternetGateway"),
	}
	// 8c. a route table
	template.Resources["RouteTable"] = &ec2.RouteTable{
		Tags:  defaultTags,
		VpcId: cloudformation.Ref("VPC"),
	}
	// 8d. a route table association
	// template.Resources["RouteTableAssociation"] = &ec2.GatewayRouteTableAssociation{
	// 	RouteTableId: cloudformation.Ref("RouteTable"),
	// 	GatewayId:    cloudformation.Ref("InternetGateway"),
	// }
	template.Resources["Route"] = &ec2.Route{
		RouteTableId:         cloudformation.Ref("RouteTable"),
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            cloudformation.RefPtr("InternetGateway"),
	}
	// 8d. a public subnet
	template.Resources["Subnet"] = &ec2.Subnet{
		Tags: defaultTags,
		// AvailabilityZone: TODO: parse region suffix
		CidrBlock:           aws.String("10.0.0.0/20"),
		VpcId:               cloudformation.Ref("VPC"),
		MapPublicIpOnLaunch: aws.Bool(true),
	}
	// 8e. a subnet route table association
	template.Resources["SubnetRouteTableAssociation"] = &ec2.SubnetRouteTableAssociation{
		SubnetId:     cloudformation.Ref("Subnet"),
		RouteTableId: cloudformation.Ref("RouteTable"),
	}
	// 8f. S3 gateway endpoint
	template.Resources["S3GatewayEndpoint"] = &ec2.VPCEndpoint{
		VpcEndpointType: aws.String("Gateway"),
		VpcId:           cloudformation.Ref("VPC"),
		ServiceName:     cloudformation.Sub("com.amazonaws.${AWS::Region}.s3"),
	}

	// Declare stack outputs
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
	template.Outputs["subnetId"] = cloudformation.Output{
		Value:       cloudformation.Ref("Subnet"),
		Description: aws.String("ID of the subnet"),
	}

	return template
}
