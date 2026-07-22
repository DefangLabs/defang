package codebuild

import (
	"context"
	"errors"
	"path"
	"strings"

	pkg "github.com/DefangLabs/defang/src/pkg"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	cbtypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"github.com/aws/smithy-go/ptr"
	"go.yaml.in/yaml/v4"
)

type buildspecDoc struct {
	Version string         `yaml:"version"`
	Phases  buildspecPhase `yaml:"phases"`
}

type buildspecPhase struct {
	Build buildspecBuild `yaml:"build"`
}

type buildspecBuild struct {
	Commands []string `yaml:"commands"`
}

// environmentTypeForImage returns the CodeBuild environment type required to run
// the given container image. The project is created as LINUX_CONTAINER (x86_64),
// but some CD images are arm64 (e.g. the legacy public-cd-image-*-arm64 tag).
// Running an arm64 image on an x86_64 environment fails at runtime with
// "node: not found" / exit 127, so the environment type must match the image.
//
// The image architecture is inferred from the reference: a sha256 digest is
// hexadecimal and cannot contain the letters in "arm64"/"aarch64", so a simple
// substring check is unambiguous. Unknown/x86_64 images keep the project default.
func environmentTypeForImage(image string) cbtypes.EnvironmentType {
	lower := strings.ToLower(image)
	if strings.Contains(lower, "arm64") || strings.Contains(lower, "aarch64") {
		return cbtypes.EnvironmentTypeArmContainer
	}
	return cbtypes.EnvironmentTypeLinuxContainer
}

func buildspec(workingDir string, cmd ...string) (string, error) {
	if workingDir == "" {
		return "", errors.New("workingDir must not be empty")
	}
	if len(cmd) == 0 {
		return "", errors.New("cmd must not be empty")
	}

	// Validate and clean the working directory path
	workingDir = path.Clean(workingDir)

	// Shell-quote the command arguments to preserve argument boundaries
	command := pkg.ShellQuote(cmd...)

	// CodeBuild overrides the image's WORKDIR; use mkdir/cd to ensure the directory exists
	shellCmd := "mkdir -p " + pkg.ShellQuote(workingDir) + " && cd " + pkg.ShellQuote(workingDir) + " && " + command

	doc := buildspecDoc{
		Version: "0.2",
		Phases: buildspecPhase{
			Build: buildspecBuild{
				Commands: []string{shellCmd},
			},
		},
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (a *AwsCodeBuild) Run(ctx context.Context, workingDir, image string, env map[string]string, cmd ...string) (BuildID, error) {
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

	spec, err := buildspec(workingDir, cmd...)
	if err != nil {
		return nil, err
	}

	client := codebuild.NewFromConfig(cfg)
	input := &codebuild.StartBuildInput{
		ProjectName:                  ptr.String(a.ProjectName),
		ImageOverride:                ptr.String(image),
		EnvironmentTypeOverride:      environmentTypeForImage(image),
		EnvironmentVariablesOverride: envOverrides,
		BuildspecOverride:            ptr.String(spec),
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
