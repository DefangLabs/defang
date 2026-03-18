package codebuild

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	cbtypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"github.com/aws/smithy-go/ptr"
)

func (a *AwsCodeBuild) Run(ctx context.Context, image string, env map[string]string, cmd ...string) (BuildID, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	var envOverrides []cbtypes.EnvironmentVariable
	for k, v := range env {
		envOverrides = append(envOverrides, cbtypes.EnvironmentVariable{
			Name:  ptr.String(k),
			Value: ptr.String(v),
		})
	}

	// Build the command to run; the buildspec executes it directly
	command := strings.Join(cmd, " ")

	// CodeBuild overrides the image's WORKDIR; use `cd` to restore it before running the command
	buildspec := "version: 0.2\nphases:\n  build:\n    commands:\n      - cd /app && " + command + "\n"

	client := codebuild.NewFromConfig(cfg)
	input := &codebuild.StartBuildInput{
		ProjectName:                  ptr.String(a.ProjectName),
		ImageOverride:                ptr.String(image),
		EnvironmentVariablesOverride: envOverrides,
		BuildspecOverride:            ptr.String(buildspec),
	}
	// Use SERVICE_ROLE credentials to pull from ECR (e.g. pull-through cache)
	if !strings.HasPrefix(image, "aws/") {
		input.ImagePullCredentialsTypeOverride = cbtypes.ImagePullCredentialsTypeServiceRole
	}
	output, err := client.StartBuild(ctx, input)
	if err != nil {
		return nil, err
	}

	return output.Build.Id, nil
}

func (a *AwsCodeBuild) Stop(ctx context.Context, buildID BuildID) error {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	client := codebuild.NewFromConfig(cfg)
	_, err = client.StopBuild(ctx, &codebuild.StopBuildInput{
		Id: buildID,
	})
	return err
}
