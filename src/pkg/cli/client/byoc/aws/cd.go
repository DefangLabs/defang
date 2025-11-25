package aws

import (
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/aws/smithy-go/ptr"
)

func makeContainers(pulumiVersion, cdImage string) []clouds.Container {
	cdSidecarName := byoc.CdTaskPrefix
	return []clouds.Container{
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
			DependsOn:  map[string]clouds.ContainerCondition{cdSidecarName: "START"},
			EntryPoint: []string{"node", "lib/index.js"},
		},
		{
			Image:     cdImage,
			Name:      cdSidecarName,
			Essential: ptr.Bool(false),
			Volumes: []clouds.TaskVolume{
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
