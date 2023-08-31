package pulumi

import (
	"context"
	"strconv"
	"strings"

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
	containerName     = "pulumi"      // TODO: should depend on image name
	ecrPublicPrefix   = "ecr-public2" // TODO: change; use project/stack name
	ecrPublicRegistry = "public.ecr.aws"
	projectName       = "projectNameX" // TODO: change
	streamPrefix      = "pulumi"       // TODO: change
)

type TaskArn = types.TaskID

type AwsEcs struct {
	stack  string
	region aws.Region
	color  Color

	spot         bool
	memoryMiB    uint32
	vCpu         float32
	image        string
	taskDefArn   string
	clusterArn   string
	logGroupName string
}

var _ types.Driver = (*AwsEcs)(nil)

func New(stack string, region aws.Region, color Color) *AwsEcs {
	return &AwsEcs{
		stack:     stack,
		region:    region,
		color:     color,
		spot:      true, // TODO: make configurable
		vCpu:      1.0,  // TODO: make configurable
		memoryMiB: 2048, // TODO: make configurable
	}
}

func (a AwsEcs) deployFunc(ctx *pulumi.Context) error {
	const PREFIX = "my-" // TODO: change to "project-stack-"

	uswest2, err := aws.NewProvider(ctx, "uswest2", &aws.ProviderArgs{
		Region: a.region,
		// SkipMetadataApiCheck: pulumi.Bool(false), NO: this stack runs locally
	})
	if err != nil {
		return err
	}

	ctx.Export("region", uswest2.Region)

	bucket, err := s3.NewBucket(ctx, PREFIX+"bucket-test", nil, pulumi.Provider(uswest2))
	if err != nil {
		return err
	}

	// ctx.Export("bucketName", bucket.ID())

	cluster, err := ecs.NewCluster(ctx, PREFIX+"cluster-test", nil, pulumi.Provider(uswest2))
	if err != nil {
		return err
	}

	ctx.Export("clusterArn", cluster.Arn)

	ptcr, err := ecr.NewPullThroughCacheRule(ctx, PREFIX+"pull-through-cache-rule-test", &ecr.PullThroughCacheRuleArgs{
		EcrRepositoryPrefix: pulumi.String(ecrPublicPrefix),
		UpstreamRegistryUrl: pulumi.String(ecrPublicRegistry),
	}, pulumi.Provider(uswest2))
	if err != nil {
		return err
	}

	image := pulumi.StringOutput(pulumi.String(a.image).ToStringOutput())
	if repo, ok := strings.CutPrefix(a.image, ecrPublicRegistry); ok {
		image = pulumi.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s%s", ptcr.RegistryId, a.region, ecrPublicPrefix, repo)
	}

	capacityProvider := "FARGATE"
	if a.spot {
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

	// Create CloudWatch log group
	logGroup, err := cloudwatch.NewLogGroup(ctx, PREFIX+"log-group-test", &cloudwatch.LogGroupArgs{
		RetentionInDays: pulumi.Int(1),
	}, pulumi.Provider(uswest2))
	if err != nil {
		return err
	}

	ctx.Export("logGroupName", logGroup.Name)

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

	containers := []map[string]any{
		{
			"name":  containerName,
			"image": image,
			"logConfiguration": map[string]any{
				// "logDriver": "awsfirelens",
				"logDriver": "awslogs",
				"options": map[string]any{
					"awslogs-group":         logGroup.ID(),
					"awslogs-region":        a.region,
					"awslogs-stream-prefix": streamPrefix,
				},
			},
			"environment": []map[string]any{ // TODO: move elsewhere
				{
					"name":  "PULUMI_BACKEND_URL",
					"value": pulumi.Sprintf("s3://%s?region=%s&awssdk=v2", bucket.ID(), a.region),
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
	containerDefJson := pulumi.JSONMarshalWithContext(ctx.Context(), containers)

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
		Memory:                  pulumi.String(strconv.FormatUint(uint64(a.memoryMiB), 10)),
		Cpu:                     pulumi.String(strconv.FormatUint(uint64(a.vCpu*1024), 10)),
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

func (a *AwsEcs) createStack(ctx context.Context) (*auto.Stack, error) {
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

func (a *AwsEcs) stackOutputs(ctx context.Context) error {
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

func (a *AwsEcs) fillOutputs(outputs auto.OutputMap) {
	a.taskDefArn = outputs["taskDefArn"].Value.(string)
	a.clusterArn = outputs["clusterArn"].Value.(string)
	a.logGroupName = outputs["logGroupName"].Value.(string)
	// a.bucketName = outputs["bucketName"].Value.(string)
	// a.subNetIds = outputs["subNetIds"].Value.([]string)
}
