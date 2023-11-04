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
	"github.com/defang-io/defang/cli/pkg/aws/ecs/cfn/outputs"
)

const createVpcResources = true

func createTemplate(image string, memory float64, vcpu float64, spot bool, arch *string) *cloudformation.Template {
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
	const _bucket = "Bucket"
	template.Resources[_bucket] = &s3.Bucket{
		Tags: defaultTags,
		// BucketName: aws.String(PREFIX + "bucket" + SUFFIX), // optional; TODO: might want to fix this name to allow Pulumi destroy after stack deletion
		AWSCloudFormationDeletionPolicy: "RetainExceptOnCreate",
	}

	// 2. ECS cluster
	const _cluster = "Cluster"
	template.Resources[_cluster] = &ecs.Cluster{
		Tags: defaultTags,
		// ClusterName: aws.String(PREFIX + "cluster" + SUFFIX), // optional
	}

	// 3. ECR pull-through cache (only really needed for images from ECR Public registry)
	// TODO: Creating pull through cache rules isn't supported in the following Regions:
	// * China (Beijing) (cn-north-1)
	// * China (Ningxia) (cn-northwest-1)
	// * AWS GovCloud (US-East) (us-gov-east-1)
	// * AWS GovCloud (US-West) (us-gov-west-1)
	const _pullThroughCache = "PullThroughCache"
	template.Resources[_pullThroughCache] = &ecr.PullThroughCacheRule{
		EcrRepositoryPrefix: aws.String(ecrPublicPrefix), // TODO: optional
		UpstreamRegistryUrl: aws.String(awsecs.EcrPublicRegistry),
	}

	// 4. ECS capacity provider
	capacityProvider := "FARGATE"
	if spot {
		capacityProvider = "FARGATE_SPOT"
	}
	const _capacityProvider = "CapacityProvider"
	template.Resources[_capacityProvider] = &ecs.ClusterCapacityProviderAssociations{
		Cluster: cloudformation.Ref(_cluster),
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
	const _logGroup = "LogGroup"
	template.Resources[_logGroup] = &logs.LogGroup{
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
	const _executionRole = "ExecutionRole"
	template.Resources[_executionRole] = &iam.Role{
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
	const _taskRole = "TaskRole"
	template.Resources[_taskRole] = &iam.Role{
		Tags: defaultTags,
		// RoleName: aws.String(PREFIX + "task-role" + SUFFIX), // optional
		ManagedPolicyArns: []string{
			"arn:aws:iam::aws:policy/AdministratorAccess", // TODO: make this configurable
		},
		AssumeRolePolicyDocument: assumeRolePolicyDocumentECS,
		Policies: []iam.Role_Policy{
			{
				// From https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-iam-roles.html#ecs-exec-required-iam-permissions
				PolicyName: "AllowExecuteCommand",
				PolicyDocument: map[string]any{
					"Version": "2012-10-17",
					"Statement": []map[string]any{
						{
							"Effect": "Allow",
							"Action": []string{
								"ssmmessages:CreateDataChannel",
								"ssmmessages:OpenDataChannel",
								"ssmmessages:OpenControlChannel",
								"ssmmessages:CreateControlChannel",
							},
							"Resource": "*", // TODO: restrict
						},
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
	const _taskDefinition = "TaskDefinition"
	template.Resources[_taskDefinition] = &ecs.TaskDefinition{
		Tags: defaultTags,
		RuntimePlatform: &ecs.TaskDefinition_RuntimePlatform{
			CpuArchitecture:       arch,
			OperatingSystemFamily: aws.String("LINUX"), // TODO: support other OSes?
		},
		ContainerDefinitions: []ecs.TaskDefinition_ContainerDefinition{
			{
				Name:  awsecs.ContainerName,
				Image: image,
				LogConfiguration: &ecs.TaskDefinition_LogConfiguration{
					LogDriver: "awslogs",
					Options: map[string]string{
						"awslogs-group":         cloudformation.Ref(_logGroup),
						"awslogs-region":        cloudformation.Ref("AWS::Region"),
						"awslogs-stream-prefix": awsecs.ProjectName,
					},
				},
				Environment: []ecs.TaskDefinition_KeyValuePair{
					{
						Name:  aws.String("PULUMI_BACKEND_URL"),                                        // TODO: this should not be here
						Value: cloudformation.SubPtr("s3://${Bucket}?region=${AWS::Region}&awssdk=v2"), // TODO: use _bucket
					},
				},
			},
		},
		Cpu:                     aws.String(strconv.FormatUint(uint64(cpu), 10)),
		ExecutionRoleArn:        cloudformation.RefPtr(_executionRole),
		Memory:                  aws.String(strconv.FormatUint(uint64(mem), 10)),
		NetworkMode:             aws.String("awsvpc"),
		RequiresCompatibilities: []string{"FARGATE"},
		TaskRoleArn:             cloudformation.RefPtr(_taskRole),
		// Family:                  cloudformation.SubPtr("${AWS::StackName}-TaskDefinition"), // optional, but needed to avoid TaskDef replacement
	}

	var vpcId *string
	if createVpcResources {
		// 8a. a VPC
		const _vpc = "VPC"
		template.Resources[_vpc] = &ec2.VPC{
			Tags:      append([]tags.Tag{{Key: "Name", Value: prefix + "vpc"}}, defaultTags...),
			CidrBlock: aws.String("10.0.0.0/16"),
		}
		vpcId = cloudformation.RefPtr(_vpc)
		// 8b. an internet gateway
		const _internetGateway = "InternetGateway"
		template.Resources[_internetGateway] = &ec2.InternetGateway{
			Tags: append([]tags.Tag{{Key: "Name", Value: prefix + "igw"}}, defaultTags...),
		}
		// 8c. an internet gateway attachment for the VPC
		const _internetGatewayAttachment = "InternetGatewayAttachment"
		template.Resources[_internetGatewayAttachment] = &ec2.VPCGatewayAttachment{
			VpcId:             cloudformation.Ref(_vpc),
			InternetGatewayId: cloudformation.RefPtr(_internetGateway),
		}
		// 8d. a route table
		const _routeTable = "RouteTable"
		template.Resources[_routeTable] = &ec2.RouteTable{
			Tags:  append([]tags.Tag{{Key: "Name", Value: prefix + "routetable"}}, defaultTags...),
			VpcId: cloudformation.Ref(_vpc),
		}
		// 8e. a route for the route table and internet gateway
		const _route = "Route"
		template.Resources[_route] = &ec2.Route{
			RouteTableId:         cloudformation.Ref(_routeTable),
			DestinationCidrBlock: aws.String("0.0.0.0/0"),
			GatewayId:            cloudformation.RefPtr(_internetGateway),
		}
		// 8f. a public subnet
		const _subnet = "Subnet"
		template.Resources[_subnet] = &ec2.Subnet{
			Tags: append([]tags.Tag{{Key: "Name", Value: prefix + "subnet"}}, defaultTags...),
			// AvailabilityZone: TODO: parse region suffix
			CidrBlock:           aws.String("10.0.0.0/20"),
			VpcId:               cloudformation.Ref(_vpc),
			MapPublicIpOnLaunch: aws.Bool(true),
		}
		// 8g. a subnet / route table association
		const _subnetRouteTableAssociation = "SubnetRouteTableAssociation"
		template.Resources[_subnetRouteTableAssociation] = &ec2.SubnetRouteTableAssociation{
			SubnetId:     cloudformation.Ref(_subnet),
			RouteTableId: cloudformation.Ref(_routeTable),
		}
		// 8h. S3 gateway endpoint (to avoid S3 bandwidth charges)
		const _s3GatewayEndpoint = "S3GatewayEndpoint"
		template.Resources[_s3GatewayEndpoint] = &ec2.VPCEndpoint{
			VpcEndpointType: aws.String("Gateway"),
			VpcId:           cloudformation.Ref(_vpc),
			ServiceName:     cloudformation.Sub("com.amazonaws.${AWS::Region}.s3"),
		}

		template.Outputs[outputs.SubnetID] = cloudformation.Output{
			Value:       cloudformation.Ref(_subnet),
			Description: aws.String("ID of the subnet"),
		}
	}

	const _securityGroup = "SecurityGroup"
	template.Resources[_securityGroup] = &ec2.SecurityGroup{
		Tags:             defaultTags, // Name tag is ignored
		GroupDescription: "Security group for the ECS task that allows all outbound and inbound traffic",
		VpcId:            vpcId, // FIXME: should be the VpcId of the given subnet
		SecurityGroupIngress: []ec2.SecurityGroup_Ingress{
			{
				IpProtocol: "tcp",
				FromPort:   aws.Int(1),
				ToPort:     aws.Int(65535),
				CidrIp:     aws.String("0.0.0.0/0"), // from anywhere FIXME: restrict to "my ip"
			},
		},
		// SecurityGroupEgress: []ec2.SecurityGroup_Egress{ FIXME: add ability to restrict outbound traffic
		// 	{
		// 		IpProtocol: "tcp",
		// 		FromPort:   aws.Int(1),
		// 		ToPort:     aws.Int(65535),
		// 		// CidrIp:     aws.String("
		// 	},
		// },
	}

	// Declare stack outputs
	template.Outputs[outputs.TaskDefArn] = cloudformation.Output{
		Value:       cloudformation.Ref(_taskDefinition),
		Description: aws.String("ARN of the ECS task definition"),
	}
	template.Outputs[outputs.ClusterArn] = cloudformation.Output{
		Value:       cloudformation.Ref(_cluster),
		Description: aws.String("ARN of the ECS cluster"),
	}
	template.Outputs[outputs.LogGroupName] = cloudformation.Output{
		Value:       cloudformation.Ref(_logGroup),
		Description: aws.String("Name of the CloudWatch log group"),
	}
	template.Outputs[outputs.SecurityGroupID] = cloudformation.Output{
		Value:       cloudformation.Ref(_securityGroup),
		Description: aws.String("ID of the security group"),
	}

	return template
}
