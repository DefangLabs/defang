package aws

import (
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/cfn"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/aws/smithy-go/ptr"
)

func makeContainers(pulumiVersion, cdImage string) []types.Container {
	cdSidecarName := byoc.CdTaskPrefix
	return []types.Container{
		{
			Image:     "public.ecr.aws/pulumi/pulumi-nodejs:" + pulumiVersion,
			Name:      ecs.CdContainerName,
			Cpus:      2.0,
			Memory:    2048_000_000, // 2G
			Essential: ptr.Bool(true),
			VolumesFrom: []string{
				cdSidecarName,
			},
			WorkDir:    "/app",
			DependsOn:  map[string]types.ContainerCondition{cdSidecarName: "START"},
			EntryPoint: []string{"node", "lib/index.js"},
		},
		{
			Image:     cdImage,
			Name:      cdSidecarName,
			Essential: ptr.Bool(false),
			Volumes: []types.TaskVolume{
				{
					Source:   "pulumi-plugins",
					Target:   "/root/.pulumi/plugins",
					ReadOnly: true,
				},
				{
					Source:   "cd",
					Target:   "/app",
					ReadOnly: true,
				},
			},
		},
	}
}

func PrintCloudFormationTemplate() ([]byte, error) {
	// TODO: grab pulumi version and cd image from Fabric CanIUse API
	containers := makeContainers("latest", "public.ecr.aws/defang-io/cd:latest")
	template, err := cfn.CreateTemplate("test-stack", containers)
	if err != nil {
		return nil, err
	}
	return template.YAML()
}
