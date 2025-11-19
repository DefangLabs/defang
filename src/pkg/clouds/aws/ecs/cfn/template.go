package cfn

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	awsecs "github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/aws/smithy-go/ptr"
	"github.com/awslabs/goformation/v7/cloudformation"
	"github.com/awslabs/goformation/v7/cloudformation/ec2"
	"github.com/awslabs/goformation/v7/cloudformation/ecr"
	"github.com/awslabs/goformation/v7/cloudformation/ecs"
	"github.com/awslabs/goformation/v7/cloudformation/iam"
	"github.com/awslabs/goformation/v7/cloudformation/logs"
	"github.com/awslabs/goformation/v7/cloudformation/policies"
	"github.com/awslabs/goformation/v7/cloudformation/s3"
	"github.com/awslabs/goformation/v7/cloudformation/secretsmanager"
	"github.com/awslabs/goformation/v7/cloudformation/tags"
)

const (
	maxCachePrefixLength = 20 // prefix must be 2-20 characters long; should be 30 https://github.com/hashicorp/terraform-provider-aws/pull/34716

	CreatedByTagKey   = "CreatedBy"
	CreatedByTagValue = awsecs.CrunProjectName
)

func getCacheRepoPrefix(prefix, suffix string) string {
	repo := prefix + suffix
	if len(repo) > maxCachePrefixLength {
		// Cache repo name is too long; hash it and use the first 6 chars
		hash := sha256.Sum256([]byte(prefix))
		return hex.EncodeToString(hash[:])[:6] + "-" + suffix
	}
	return repo
}

const TemplateRevision = 2 // bump this when the template changes!

// CreateStaticTemplate creates a parameterized CloudFormation template that can be statically served
// All conditional logic is moved to CloudFormation parameters and conditions.
// This allows the template to be generated once and reused across different deployments
// by providing different parameter values during stack creation/update.
//
// Parameters supported:
// - UseSpotInstances: "true"/"false" - Whether to use FARGATE_SPOT capacity provider
// - ExistingVpcId: VPC ID string or empty to create new VPC
// - RetainBucket: "true"/"false" - Whether to retain S3 bucket on stack deletion
// - EnablePullThroughCache: "true"/"false" - Whether to enable ECR pull-through cache
// - DockerHubUsername: Username for Docker Hub authentication (optional)
// - DockerHubAccessToken: Access token for Docker Hub authentication (optional)
func createTemplate(stack string, containers []types.Container) (*cloudformation.Template, error) {
	prefix := stack + "-"

	defaultTags := []tags.Tag{
		{
			Key:   CreatedByTagKey,
			Value: CreatedByTagValue,
		},
	}

	template := cloudformation.NewTemplate()
	template.Description = "Defang AWS CloudFormation template for an ECS task. Don't delete: use the CLI instead."

	// Parameters
	template.Parameters[ParamsUseSpotInstances] = cloudformation.Parameter{
		Type:          "String",
		Default:       ptr.String("false"),
		AllowedValues: []any{"true", "false"},
		Description:   ptr.String("Whether to use FARGATE_SPOT capacity provider"),
	}
	template.Parameters[ParamsExistingVpcId] = cloudformation.Parameter{
		Type:        "String",
		Default:     ptr.String(""),
		Description: ptr.String("ID of existing VPC to use (leave empty to create new VPC)"),
	}
	template.Parameters[ParamsRetainBucket] = cloudformation.Parameter{
		Type:          "String",
		Default:       ptr.String("true"),
		AllowedValues: []any{"true", "false"},
		Description:   ptr.String("Whether to retain the S3 bucket on stack deletion"),
	}
	template.Parameters[ParamsEnablePullThroughCache] = cloudformation.Parameter{
		Type:          "String",
		Default:       ptr.String("true"),
		AllowedValues: []any{"true", "false"},
		Description:   ptr.String("Whether to enable ECR pull-through cache"),
	}
	template.Parameters[ParamsDockerHubUsername] = cloudformation.Parameter{
		Type:        "String",
		Default:     ptr.String(""),
		Description: ptr.String("Docker Hub username for private registry access (optional)"),
		NoEcho:      ptr.Bool(true),
	}
	template.Parameters[ParamsDockerHubAccessToken] = cloudformation.Parameter{
		Type:        "String",
		Default:     ptr.String(""),
		Description: ptr.String("Docker Hub access token for private registry access (optional)"),
		NoEcho:      ptr.Bool(true),
	}

	// Conditions
	const _condUseSpot = "UseSpot"
	template.Conditions[_condUseSpot] = cloudformation.Equals(cloudformation.Ref(ParamsUseSpotInstances), "true")
	const _condCreateVpcResources = "CreateVpcResources"
	template.Conditions[_condCreateVpcResources] = cloudformation.Equals(cloudformation.Ref(ParamsExistingVpcId), "")
	const _condRetainS3Bucket = "RetainS3Bucket"
	template.Conditions[_condRetainS3Bucket] = cloudformation.Equals(cloudformation.Ref(ParamsRetainBucket), "true")
	const _condEnablePullThroughCache = "EnablePullThroughCache"
	template.Conditions[_condEnablePullThroughCache] = cloudformation.Equals(cloudformation.Ref(ParamsEnablePullThroughCache), "true")
	const _condEnableDockerPullThroughCache = "EnableDockerPullThroughCache"
	template.Conditions[_condEnableDockerPullThroughCache] = cloudformation.And([]string{
		cloudformation.Equals(cloudformation.Ref(ParamsEnablePullThroughCache), "true"),
		cloudformation.Not([]string{cloudformation.Equals(cloudformation.Ref(ParamsDockerHubUsername), "")}),
		cloudformation.Not([]string{cloudformation.Equals(cloudformation.Ref(ParamsDockerHubAccessToken), "")}),
	})

	// 1. bucket (for deployment state)
	const _bucket = "Bucket"
	template.Resources[_bucket] = &s3.Bucket{
		Tags: defaultTags,
		// BucketName: ptr.String(PREFIX + "bucket" + SUFFIX), // optional; TODO: might want to fix this name to allow Pulumi destroy after stack deletion
		AWSCloudFormationDeletionPolicy: policies.DeletionPolicy(cloudformation.If(_condRetainS3Bucket, "RetainExceptOnCreate", "Delete")),
		VersioningConfiguration: &s3.Bucket_VersioningConfiguration{
			Status: "Enabled",
		},
	}

	// 2. ECS cluster
	const _cluster = "Cluster"
	template.Resources[_cluster] = &ecs.Cluster{
		Tags: defaultTags,
		// ClusterName: ptr.String(PREFIX + "cluster" + SUFFIX), // optional
	}

	// 3. ECS capacity provider
	const _capacityProvider = "CapacityProvider"
	template.Resources[_capacityProvider] = &ecs.ClusterCapacityProviderAssociations{
		Cluster: cloudformation.Ref(_cluster),
		CapacityProviders: []string{
			cloudformation.If(_condUseSpot, "FARGATE_SPOT", "FARGATE"),
		},
		DefaultCapacityProviderStrategy: []ecs.ClusterCapacityProviderAssociations_CapacityProviderStrategy{
			{
				CapacityProvider: cloudformation.If(_condUseSpot, "FARGATE_SPOT", "FARGATE"),
				Weight:           ptr.Int(1),
			},
		},
	}

	// 4. CloudWatch log group
	const _logGroup = "LogGroup"
	template.Resources[_logGroup] = &logs.LogGroup{
		Tags: defaultTags,
		// LogGroupName:    ptr.String(PREFIX + "log-group-test" + SUFFIX), // optional
		RetentionInDays: ptr.Int(1),
		// Make sure the log group cannot be deleted while the cluster is up
		AWSCloudFormationDependsOn: []string{
			_cluster,
		},
	}

	// 5. ECR pull-through cache rules
	// TODO: Creating pull through cache rules isn't supported in the following Regions:
	// * China (Beijing) (cn-north-1)
	// * China (Ningxia) (cn-northwest-1)
	// * AWS GovCloud (US-East) (us-gov-east-1)
	// * AWS GovCloud (US-West) (us-gov-west-1)

	// Create pull-through cache resources conditionally
	ecrPublicPrefix := getCacheRepoPrefix(prefix, "ecr-public")
	dockerPublicPrefix := getCacheRepoPrefix(prefix, "docker-public")

	// 5a. ECR Public pull-through cache
	const _pullThroughCache = "PullThroughCache"
	template.Resources[_pullThroughCache] = &ecr.PullThroughCacheRule{
		AWSCloudFormationCondition: _condEnablePullThroughCache,
		EcrRepositoryPrefix:        ptr.String(ecrPublicPrefix),
		UpstreamRegistryUrl:        ptr.String(awsecs.EcrPublicRegistry),
	}

	// 5b. Docker Hub credentials secret - needs proper JSON format
	// When creating the Secrets Manager secret that contains the upstream registry credentials, the secret name must use the `ecr-pullthroughcache/` prefix.
	// This is the struct AWS wants, see https://docs.aws.amazon.com/AmazonECR/latest/userguide/pull-through-cache-creating-secret.html
	// #nosec G101 - not a secret
	const _privateRepoSecret = "PrivateRepoSecret"
	template.Resources[_privateRepoSecret] = &secretsmanager.Secret{
		AWSCloudFormationCondition: _condEnableDockerPullThroughCache,
		Tags:                       defaultTags,
		Description:                ptr.String("Docker Hub credentials for the ECR pull-through cache rule"),
		Name:                       ptr.String("ecr-pullthroughcache/" + dockerPublicPrefix),
		SecretString:               ptr.String(cloudformation.Sub(`{"username":"${` + ParamsDockerHubUsername + `}","accessToken":"${` + ParamsDockerHubAccessToken + `}"}`)),
	}

	// 5c. Docker Hub pull-through cache
	const _pullThroughCacheDocker = "PullThroughCacheDocker"
	template.Resources[_pullThroughCacheDocker] = &ecr.PullThroughCacheRule{
		AWSCloudFormationCondition: _condEnableDockerPullThroughCache,
		EcrRepositoryPrefix:        ptr.String(dockerPublicPrefix),
		UpstreamRegistryUrl:        ptr.String("registry-1.docker.io"),
		CredentialArn:              cloudformation.RefPtr(_privateRepoSecret),
	}

	// 6. IAM roles for ECS task
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

	// 6a. IAM exec role for task
	execPolicies := []iam.Role_Policy{
		{
			// From https://docs.aws.amazon.com/AmazonECR/latest/userguide/pull-through-cache.html#pull-through-cache-iam
			PolicyName: "AllowECRPassThrough",
			PolicyDocument: map[string]any{
				"Version": "2012-10-17",
				"Statement": []any{
					map[string]any{
						"Effect": "Allow",
						"Action": []string{
							"ecr:CreatePullThroughCacheRule",
							"ecr:BatchImportUpstreamImage", // should be registry permission instead
							"ecr:CreateRepository",         // can be registry permission instead
						},
						"Resource": "*", // FIXME: restrict cloudformation.Sub("arn:${AWS::Partition}:ecr:${AWS::Region}:${AWS::AccountId}:repository/${PullThroughCache}:*"),
					},
					cloudformation.If(_condEnableDockerPullThroughCache,
						map[string]any{
							"Effect": "Allow",
							"Action": []string{
								"secretsmanager:GetSecretValue",
								"ssm:GetParameters",
								// "kms:Decrypt", Required only if your key uses a custom KMS key and not the default key
							},
							"Resource": cloudformation.Ref(_privateRepoSecret),
						},
						cloudformation.Ref("AWS::NoValue"),
					),
				},
			},
		},
	}

	const _executionRole = "ExecutionRole"
	template.Resources[_executionRole] = &iam.Role{
		Tags: defaultTags,
		// RoleName: ptr.String(PREFIX + "execution-role" + SUFFIX), // optional
		ManagedPolicyArns: []string{
			"arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy",
		},
		AssumeRolePolicyDocument: assumeRolePolicyDocumentECS,
		Policies:                 execPolicies,
	}

	// 6b. IAM role for task (optional)
	const _taskRole = "TaskRole"
	template.Resources[_taskRole] = &iam.Role{
		Tags: defaultTags,
		// RoleName: ptr.String(PREFIX + "task-role" + SUFFIX), // optional
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
			{
				PolicyName: "AllowPassRole",
				PolicyDocument: map[string]any{
					"Version": "2012-10-17",
					"Statement": []map[string]any{
						{
							"Effect": "Allow",
							"Action": []string{
								"iam:PassRole",
							},
							"Resource": "*", // TODO: restrict to roles that are needed/created by the task
						},
					},
				},
			},
			{
				PolicyName: "AllowAssumeRole",
				PolicyDocument: map[string]any{
					"Version": "2012-10-17",
					"Statement": []map[string]any{
						{
							"Effect": "Allow",
							"Action": []string{
								"sts:AssumeRole",
							},
							"Resource": "*",
						},
					},
				},
			},
		},
	}

	// 7. ECS task definition
	var totalCpu, totalMiB float64
	var platform string
	for _, container := range containers {
		totalCpu += float64(container.Cpus)
		totalMiB += math.Max(float64(container.Memory)/1024/1024, 6) // 6MiB min for the container
		if platform == "" {
			platform = container.Platform
		} else if platform != container.Platform {
			return nil, errors.New("all containers must have the same platform")
		}
	}
	mCpu, mib := awsecs.FixupFargateConfig(totalCpu, totalMiB)
	arch, os := awsecs.PlatformToArchOS(platform)
	var archP, osP *string
	if arch != "" {
		archP = ptr.String(arch)
	}
	if os != "" {
		osP = ptr.String(os)
	}

	var volumes []ecs.TaskDefinition_Volume
	var containerDefinitions []ecs.TaskDefinition_ContainerDefinition
	for _, container := range containers {
		for _, v := range container.Volumes {
			volumes = append(volumes, ecs.TaskDefinition_Volume{
				Name: ptr.String(v.Source),
			})
		}

		volumesFrom := make([]ecs.TaskDefinition_VolumeFrom, 0, len(container.VolumesFrom))
		for _, v := range container.VolumesFrom {
			parts := strings.SplitN(v, ":", 2)
			ro := false
			if len(parts) == 2 && parts[1] == "ro" {
				ro = true
			}
			volumesFrom = append(volumesFrom, ecs.TaskDefinition_VolumeFrom{
				ReadOnly:        ptr.Bool(ro),
				SourceContainer: ptr.String(parts[0]),
			})
		}

		mountPoints := make([]ecs.TaskDefinition_MountPoint, 0, len(container.Volumes))
		for _, v := range container.Volumes {
			mountPoints = append(mountPoints, ecs.TaskDefinition_MountPoint{
				ContainerPath: ptr.String(v.Target),
				SourceVolume:  ptr.String(v.Source),
				ReadOnly:      ptr.Bool(v.ReadOnly),
			})
		}

		var cpuShares *int
		if container.Cpus > 0 {
			cpuShares = ptr.Int(int(container.Cpus * 1024))
		}
		name := container.Name
		if name == "" {
			name = awsecs.CdContainerName // TODO: backwards compat; remove this
		}

		var dependsOn []ecs.TaskDefinition_ContainerDependency
		if container.DependsOn != nil {
			for name, condition := range container.DependsOn {
				dependsOn = append(dependsOn, ecs.TaskDefinition_ContainerDependency{
					Condition:     ptr.String(string(condition)),
					ContainerName: ptr.String(name),
				})
			}
		}

		image := container.Image
		if repo, ok := strings.CutPrefix(image, awsecs.EcrPublicRegistry); ok {
			image = cloudformation.If(_condEnablePullThroughCache,
				cloudformation.Sub("${AWS::AccountId}.dkr.ecr.${AWS::Region}.amazonaws.com/"+ecrPublicPrefix+repo),
				container.Image,
			)
		} else if repo, ok := strings.CutPrefix(image, awsecs.DockerRegistry); ok {
			image = cloudformation.If(_condEnableDockerPullThroughCache,
				cloudformation.Sub("${AWS::AccountId}.dkr.ecr.${AWS::Region}.amazonaws.com/"+dockerPublicPrefix+repo),
				container.Image,
			)
		} else {
			// TODO: support pull through cache for other registries
			// TODO: support private repos (with or without pull-through cache)
		}
		if image == "" {
			return nil, fmt.Errorf("container %v is using invalid image: %q", container.Name, image)
		}

		def := ecs.TaskDefinition_ContainerDefinition{
			Name:        name,
			Image:       image,
			StopTimeout: ptr.Int(120), // TODO: make this configurable
			Essential:   container.Essential,
			Cpu:         cpuShares,
			LogConfiguration: &ecs.TaskDefinition_LogConfiguration{
				LogDriver: "awslogs",
				Options: map[string]string{
					"awslogs-group":         cloudformation.Ref(_logGroup),
					"awslogs-region":        cloudformation.Ref("AWS::Region"),
					"awslogs-stream-prefix": awsecs.AwsLogsStreamPrefix,
				},
			},
			VolumesFrom:   volumesFrom,
			MountPoints:   mountPoints,
			EntryPoint:    container.EntryPoint,
			Command:       container.Command,
			DependsOnProp: dependsOn,
		}
		if container.WorkDir != "" {
			def.WorkingDirectory = ptr.String(container.WorkDir)
		}
		containerDefinitions = append(containerDefinitions, def)
	}

	const _taskDefinition = "TaskDefinition"
	template.Resources[_taskDefinition] = &ecs.TaskDefinition{
		Tags: defaultTags,
		RuntimePlatform: &ecs.TaskDefinition_RuntimePlatform{
			CpuArchitecture:       archP,
			OperatingSystemFamily: osP,
		},
		Volumes:                 volumes,
		ContainerDefinitions:    containerDefinitions,
		Cpu:                     ptr.String(strconv.FormatUint(uint64(mCpu), 10)), // MilliCPU
		ExecutionRoleArn:        cloudformation.RefPtr(_executionRole),
		Memory:                  ptr.String(strconv.FormatUint(uint64(mib), 10)), // MiB
		NetworkMode:             ptr.String("awsvpc"),
		RequiresCompatibilities: []string{"FARGATE"},
		TaskRoleArn:             cloudformation.RefPtr(_taskRole),
		// Family:                  cloudformation.SubPtr("${AWS::StackName}-TaskDefinition"), // optional, but needed to avoid TaskDef replacement
	}

	// VPC resources - create conditionally
	const _vpc = "VPC"
	template.Resources[_vpc] = &ec2.VPC{
		AWSCloudFormationCondition: _condCreateVpcResources,
		Tags:                       append([]tags.Tag{{Key: "Name", Value: prefix + "vpc"}}, defaultTags...),
		CidrBlock:                  ptr.String("10.0.0.0/16"),
	}

	vpcId := cloudformation.If(_condCreateVpcResources, cloudformation.Ref(_vpc), cloudformation.Ref(ParamsExistingVpcId))
	// 8b. an internet gateway; TODO: make internet access optional
	const _internetGateway = "InternetGateway"
	template.Resources[_internetGateway] = &ec2.InternetGateway{
		AWSCloudFormationCondition: _condCreateVpcResources,
		Tags:                       append([]tags.Tag{{Key: "Name", Value: prefix + "igw"}}, defaultTags...),
	}
	// 8c. an internet gateway attachment for the VPC
	const _internetGatewayAttachment = "InternetGatewayAttachment"
	template.Resources[_internetGatewayAttachment] = &ec2.VPCGatewayAttachment{
		AWSCloudFormationCondition: _condCreateVpcResources,
		VpcId:                      cloudformation.Ref(_vpc),
		InternetGatewayId:          cloudformation.RefPtr(_internetGateway),
	}
	// 8d. a route table
	const _routeTable = "RouteTable"
	template.Resources[_routeTable] = &ec2.RouteTable{
		AWSCloudFormationCondition: _condCreateVpcResources,
		Tags:                       append([]tags.Tag{{Key: "Name", Value: prefix + "routetable"}}, defaultTags...),
		VpcId:                      cloudformation.Ref(_vpc),
	}
	// 8e. a route for the route table and internet gateway
	const _route = "Route"
	template.Resources[_route] = &ec2.Route{
		AWSCloudFormationCondition: _condCreateVpcResources,
		RouteTableId:               cloudformation.Ref(_routeTable),
		DestinationCidrBlock:       ptr.String("0.0.0.0/0"),
		GatewayId:                  cloudformation.RefPtr(_internetGateway),
	}
	// 8f. a public subnet
	const _subnet = "Subnet"
	template.Resources[_subnet] = &ec2.Subnet{
		AWSCloudFormationCondition: _condCreateVpcResources,
		Tags:                       append([]tags.Tag{{Key: "Name", Value: prefix + "subnet"}}, defaultTags...),
		// AvailabilityZone:; TODO: parse region suffix
		CidrBlock:           ptr.String("10.0.0.0/20"),
		VpcId:               cloudformation.Ref(_vpc),
		MapPublicIpOnLaunch: ptr.Bool(true),
	}
	// 8g. a subnet / route table association
	const _subnetRouteTableAssociation = "SubnetRouteTableAssociation"
	template.Resources[_subnetRouteTableAssociation] = &ec2.SubnetRouteTableAssociation{
		AWSCloudFormationCondition: _condCreateVpcResources,
		SubnetId:                   cloudformation.Ref(_subnet),
		RouteTableId:               cloudformation.Ref(_routeTable),
	}
	// 8h. S3 gateway endpoint (to avoid S3 bandwidth charges)
	const _s3GatewayEndpoint = "S3GatewayEndpoint"
	template.Resources[_s3GatewayEndpoint] = &ec2.VPCEndpoint{
		AWSCloudFormationCondition: _condCreateVpcResources,
		VpcEndpointType:            ptr.String("Gateway"),
		VpcId:                      cloudformation.Ref(_vpc),
		ServiceName:                cloudformation.Sub("com.amazonaws.${AWS::Region}.s3"),
	}

	const _defaultSecurityGroup = "DefaultSecurityGroup"
	template.Outputs[OutputsDefaultSecurityGroupID] = cloudformation.Output{
		Condition:   ptr.String(_condCreateVpcResources),
		Description: ptr.String("ID of the default security group"),
		Value:       cloudformation.GetAtt(_vpc, _defaultSecurityGroup),
	}
	template.Outputs[OutputsSubnetID] = cloudformation.Output{
		Condition:   ptr.String(_condCreateVpcResources),
		Value:       cloudformation.Ref(_subnet),
		Description: ptr.String("ID of the subnet"),
	}

	const _securityGroup = "SecurityGroup"
	template.Resources[_securityGroup] = &ec2.SecurityGroup{
		Tags:             defaultTags, // Name tag is ignored
		GroupDescription: "Security group for the ECS task that allows all outbound and inbound traffic",
		VpcId:            ptr.String(vpcId),
		// SecurityGroupEgress: []ec2.SecurityGroup_Egress{; use default egress; FIXME: add ability to restrict outbound traffic
		// 	{
		// 		IpProtocol: "tcp",
		// 		FromPort:   ptr.Int(1),
		// 		ToPort:     ptr.Int(65535),
		// 		// CidrIp:     ptr.String("
		// 	},
		// },
	}

	// Declare the remaining stack outputs
	template.Outputs[OutputsTaskDefArn] = cloudformation.Output{
		Description: ptr.String("ARN of the ECS task definition"),
		Value:       cloudformation.Ref(_taskDefinition),
	}
	template.Outputs[OutputsClusterName] = cloudformation.Output{
		Description: ptr.String("Name of the ECS cluster"),
		Value:       cloudformation.Ref(_cluster),
	}
	template.Outputs[OutputsLogGroupARN] = cloudformation.Output{
		Description: ptr.String("ARN of the CloudWatch log group"),
		Value:       cloudformation.GetAtt(_logGroup, "Arn"),
	}
	template.Outputs[OutputsSecurityGroupID] = cloudformation.Output{
		Description: ptr.String("ID of the security group"),
		Value:       cloudformation.Ref(_securityGroup),
	}
	template.Outputs[OutputsBucketName] = cloudformation.Output{
		Description: ptr.String("Name of the S3 bucket"),
		Value:       cloudformation.Ref(_bucket),
	}
	template.Outputs[OutputsTemplateVersion] = cloudformation.Output{
		Description: ptr.String("Version of this CloudFormation template"),
		Value:       cloudformation.Int(TemplateRevision),
	}

	return template, nil
}
