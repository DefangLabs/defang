package pulumi

import (
	"context"
	"strconv"
	"strings"

	awsecs "github.com/defang-io/defang/cli/pkg/aws/ecs"
	"github.com/defang-io/defang/cli/pkg/types"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ecr"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	projectName = "projectNameX" // TODO: change
)

type TaskArn = types.TaskID

type PulumiEcs struct {
	awsecs.AwsEcs
	image  string
	stack  string
	color  Color
	memory uint64
}

var _ types.Driver = (*PulumiEcs)(nil)

func New(stack string, region aws.Region, color Color) *PulumiEcs {
	return &PulumiEcs{
		stack: stack,
		color: color,
		AwsEcs: awsecs.AwsEcs{
			Region: region,
			Spot:   true,
			VCpu:   1.0,
		},
	}
}

func (a PulumiEcs) deployFunc(ctx *pulumi.Context) error {
	const PREFIX = "my-"                  // TODO: change to "project-stack-"
	const ecrPublicPrefix = "ecr-public2" // TODO: change; use project/stack name

	// 0. AWS provider (we disabled the default providers)
	uswest2, err := aws.NewProvider(ctx, "uswest2", &aws.ProviderArgs{ // TODO: misnomer
		Region: a.Region,
		// SkipMetadataApiCheck: pulumi.Bool(false), NO: this stack runs locally
	})
	if err != nil {
		return err
	}

	ctx.Export("region", uswest2.Region)

	// 1. bucket (for state)
	bucket, err := s3.NewBucket(ctx, PREFIX+"bucket-test", nil, pulumi.Provider(uswest2))
	if err != nil {
		return err
	}

	// ctx.Export("bucketName", bucket.ID())

	// 2. ECS cluster
	cluster, err := ecs.NewCluster(ctx, PREFIX+"cluster-test", nil, pulumi.Provider(uswest2))
	if err != nil {
		return err
	}

	ctx.Export("clusterArn", cluster.Arn)

	// 3. ECR pull-through cache
	ptcr, err := ecr.NewPullThroughCacheRule(ctx, PREFIX+"pull-through-cache-rule-test", &ecr.PullThroughCacheRuleArgs{
		EcrRepositoryPrefix: pulumi.String(ecrPublicPrefix),
		UpstreamRegistryUrl: pulumi.String(awsecs.EcrPublicRegistry),
	}, pulumi.Provider(uswest2))
	if err != nil {
		return err
	}

	image := pulumi.StringOutput(pulumi.String(a.image).ToStringOutput())
	if repo, ok := strings.CutPrefix(a.image, awsecs.EcrPublicRegistry); ok {
		image = pulumi.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s%s", ptcr.RegistryId, a.Region, ecrPublicPrefix, repo)
	}

	// 4. ECS capacity provider
	capacityProvider := "FARGATE"
	if a.Spot {
		capacityProvider = "FARGATE_SPOT"
	}
	_, err = ecs.NewClusterCapacityProviders(ctx, PREFIX+"cluster-capacity-providers-test", &ecs.ClusterCapacityProvidersArgs{
		ClusterName: cluster.Name,
		CapacityProviders: pulumi.StringArray{
			pulumi.String(capacityProvider),
		},
		DefaultCapacityProviderStrategies: ecs.ClusterCapacityProvidersDefaultCapacityProviderStrategyArray{
			&ecs.ClusterCapacityProvidersDefaultCapacityProviderStrategyArgs{
				CapacityProvider: pulumi.String(capacityProvider),
				Weight:           pulumi.Int(1),
			},
		},
	}, pulumi.Provider(uswest2))
	if err != nil {
		return err
	}

	// 5. CloudWatch log group
	logGroup, err := cloudwatch.NewLogGroup(ctx, PREFIX+"log-group-test", &cloudwatch.LogGroupArgs{
		RetentionInDays: pulumi.Int(1),
	}, pulumi.Provider(uswest2))
	if err != nil {
		return err
	}

	ctx.Export("logGroupName", logGroup.Name)

	// 6. IAM role for task
	powerUserRole, err := iam.NewRole(ctx, PREFIX+"task-role-test", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(`{
			"Version": "2012-10-17",
			"Statement": [
			  {
				"Effect": "Allow",
				"Principal": {
				  "Service": "ecs-tasks.amazonaws.com"
				},
				"Action": "sts:AssumeRole"
			  }
			]
		  }`),
		ManagedPolicyArns: pulumi.StringArray{iam.ManagedPolicyPowerUserAccess}, // FIXME: too much
	}, pulumi.Provider(uswest2))
	if err != nil {
		return err
	}

	containers := a.createContainerDefinition(image, logGroup.ID(), bucket.ID())
	containerDefJson := pulumi.JSONMarshalWithContext(ctx.Context(), containers)

	// 7. ECS task definition
	cpu, mem := awsecs.FixupFargateConfig(a.VCpu, float64(a.memory)/1024/1024)
	td, err := ecs.NewTaskDefinition(ctx, PREFIX+"task-def-test", &ecs.TaskDefinitionArgs{
		// RuntimePlatform: ecs.TaskDefinitionRuntimePlatformArgs{
		// 	OperatingSystemFamily: pulumi.String("LINUX"),
		// 	CpuArchitecture: 	 pulumi.String("X86_64"),
		// },
		// Volumes: ecs.TaskDefinitionVolumeArray{
		// 	ecs.TaskDefinitionVolumeArgs{Name: pulumi.String("files")},
		// },
		TaskRoleArn:             powerUserRole.Arn, // ??? FIXME: too much?
		ExecutionRoleArn:        powerUserRole.Arn, // needed for "awslogs"; FIXME: too much?
		NetworkMode:             pulumi.String("awsvpc"),
		RequiresCompatibilities: pulumi.StringArray{pulumi.String("FARGATE")},
		Memory:                  pulumi.String(strconv.FormatUint(uint64(mem), 10)),
		Cpu:                     pulumi.String(strconv.FormatUint(uint64(cpu), 10)),
		Family:                  pulumi.String(PREFIX + "task-def-test"),
		ContainerDefinitions:    containerDefJson,
	}, pulumi.Provider(uswest2))
	if err != nil {
		return err
	}

	ctx.Export("taskDefArn", td.Arn)
	// ctx.Export("subNetIds", aws.(ctx, &aws.GetSubnetIdsArgs{}).Ids)
	return nil
}

func (a PulumiEcs) createContainerDefinition(image, logGroupID, bucketID pulumi.Input) any {
	return []map[string]any{
		{
			"name":  awsecs.ContainerName,
			"image": image,
			"logConfiguration": map[string]any{
				// "logDriver": "awsfirelens",
				"logDriver": "awslogs",
				"options": map[string]any{
					"awslogs-group":         logGroupID,
					"awslogs-region":        a.Region,
					"awslogs-stream-prefix": awsecs.StreamPrefix,
				},
			},
			"environment": []map[string]any{ // TODO: move elsewhere
				{
					"name":  "PULUMI_BACKEND_URL",
					"value": pulumi.Sprintf("s3://%s?region=%s&awssdk=v2", bucketID, a.Region),
				},
				{
					"name":  "PULUMI_SKIP_UPDATE_CHECK",
					"value": "true",
				},
				{
					"name":  "PULUMI_SKIP_CONFIRMATIONS",
					"value": "true",
				},
			},
			// "dependsOn": []map[string]any{{
			// 	"containerName": logRouterName,
			// 	"condition":     "START",
			// }},
			// "volumesFrom": []map[string]any{{"sourceContainer": "files"}},
		},
		// {
		// 	"name":      "files",
		// 	"image":     "blah",
		// 	"essential": false,
		// },
		// {
		// 	"name":  logRouterName,
		// 	"image": "public.ecr.aws/aws-observability/aws-for-fluent-bit:latest",
		// 	"firelensConfiguration": map[string]any{
		// 		"type": "fluentbit",
		// 		// "options": map[string]any{"enable-ecs-log-metadata": "true"},
		// 	},
		// 	"command": []string{
		// 		"/bin/sh", // need to use /bin/sh to use $HOSTNAME (as part of the tag), $NATS_HOST, and $TENANT_ID.$SERVICE_NAME.$SERVICE_VERSION
		// 		"-c",
		// 		// This script uses parameter expansion with the %% and # operators to remove the shortest and longest matching substrings, respectively.
		// 		`exec /fluent-bit/bin/fluent-bit -F modify -p "Add=host \${HOSTNAME%%.*}" -m '*' -o loki -m '*' -p host=127.0.0.1 -p port=3100 -p labels="container=\\$container_name" -c /fluent-bit/etc/fluent-bit.conf -v`,
		// 	},
		// 	"dependsOn": []map[string]any{{
		// 		"containerName": "loki",
		// 		"condition":     "START",
		// 	}},
		// },
		// {
		// 	"name":  "loki",
		// 	"image": "docker.io/grafana/loki:latest",
		// 	"portMappings": []map[string]any{
		// 		{"containerPort": 3100, "protocol": "tcp"}, // http
		// 		{"containerPort": 9095, "protocol": "tcp"}, // grpc
		// 	},
		// },
	}
}

func (a *PulumiEcs) createStack(ctx context.Context) (*auto.Stack, error) {
	s, err := auto.UpsertStackInlineSource(ctx, a.stack, projectName, a.deployFunc)
	if err != nil {
		return nil, err
	}

	if err := s.Workspace().InstallPlugin(ctx, "aws", "v4.0.0"); err != nil {
		return nil, err
	}

	// Disable all default providers
	if err := s.SetConfig(ctx, "pulumi:disable-default-providers", auto.ConfigValue{Value: `["*"]`}); err != nil {
		return nil, err
	}

	return &s, nil
}

func (a *PulumiEcs) Refresh(ctx context.Context) error {
	s, err := a.createStack(ctx)
	if err != nil {
		return err
	}

	o, err := s.Outputs(ctx)
	if err != nil {
		return err
	}

	a.fillOutputs(o)
	return nil
}

func (a *PulumiEcs) fillOutputs(outputs auto.OutputMap) {
	a.TaskDefArn = outputs["taskDefArn"].Value.(string)
	a.ClusterArn = outputs["clusterArn"].Value.(string)
	a.LogGroupName = outputs["logGroupName"].Value.(string)
	// a.BucketName = outputs["bucketName"].Value.(string)
	// a.SubNetIds = outputs["subNetIds"].Value.([]string)
}

func (a *PulumiEcs) Run(ctx context.Context, env map[string]string, cmd ...string) (TaskArn, error) {
	if err := a.Refresh(ctx); err != nil {
		return nil, err
	}
	return a.AwsEcs.Run(ctx, env, cmd...)
}

func (a *PulumiEcs) Tail(ctx context.Context, task TaskArn) error {
	if err := a.Refresh(ctx); err != nil {
		return err
	}
	return a.AwsEcs.Tail(ctx, task)
}
