package compose

import (
	"bufio"
	"maps"
	"os"
	"slices"
	"strings"

	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func FindAllBaseImages(project *composeTypes.Project) ([]string, error) {
	baseImages := make(map[string]struct{})
	for _, service := range project.Services {
		if service.Build != nil && service.Build.Context != "" {
			dockerfilePath := service.Build.Dockerfile
			if dockerfilePath == "" {
				dockerfilePath = "Dockerfile"
			}
			dockerfileFullPath := service.Build.Context + string(os.PathSeparator) + dockerfilePath
			images, err := extractDockerfileBaseImages(dockerfileFullPath)
			if err != nil {
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

	var images []string
	sc := bufio.NewScanner(f)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(strings.ToUpper(line), "FROM ") {
			img := strings.TrimSpace(line[5:])
			// remove AS part
			if idx := strings.Index(strings.ToUpper(img), " AS "); idx != -1 {
				img = strings.TrimSpace(img[:idx])
			}
			images = append(images, img)
		}
	}
	return images, sc.Err()
}
