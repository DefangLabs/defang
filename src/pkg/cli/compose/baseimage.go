package compose

import (
"filepath"
"fmt"
"maps"
"os"
"slices"

	"github.com/DefangLabs/defang/src/pkg/term"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

func FindAllBaseImages(project *composeTypes.Project) ([]string, error) {
	baseImages := make(map[string]struct{})
	for _, service := range project.Services {
		if service.Build != nil && service.Build.Context != "" {
			dockerfilePath := service.Build.Dockerfile
			if dockerfilePath == "" {
				dockerfilePath = "Dockerfile"
			}
			dockerfileFullPath := filepath.Join(service.Build.Context, dockerfilePath)
			images, err := extractDockerfileBaseImages(dockerfileFullPath)
			if err != nil {
				if os.IsNotExist(err) {
					term.Debugf("service %q: dockerfile %q does not exist; skipping", service.Name, dockerfileFullPath)
					continue
				}
				return nil, err
			}
			for _, img := range images {
				baseImages[img] = struct{}{}
			}
		} else if service.Image != "" {
			baseImages[service.Image] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(baseImages)), nil
}

func extractDockerfileBaseImages(dockerfilePath string) ([]string, error) {
	f, err := os.Open(dockerfilePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	result, err := parser.Parse(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Dockerfile: %w", err)
	}

	stages, metaArgs, err := instructions.Parse(result.AST, nil) // 2nd param is linter, can be nil
	if err != nil {
		return nil, fmt.Errorf("failed to parse instructions: %w", err)
	}

	// TODO: use metaArgs to resolve ARGs in FROM statements
	_ = metaArgs

	var images []string
	for _, s := range stages {
		images = append(images, s.BaseName)
	}

	return images, nil
}
