package appPlatform

import (
	"fmt"
	"regexp"
)

// Copied from defang-mvp/pulumi/shared/utils.ts
var IMG_RE = regexp.MustCompile(`^(?:(.{1,127}?)\/)?(.{1,127}?)(?::(\w[\w.-]{0,127}))?(?:@(sha256:[0-9a-f]{64}))?$`)

type Image struct {
	Registry string
	Repo     string
	Tag      string
	Digest   string
}

func ParseImage(image string) (*Image, error) {
	parts := IMG_RE.FindStringSubmatch(image)
	if parts == nil {
		return nil, fmt.Errorf("invalid image: %s", image)
	}
	return &Image{
		Registry: parts[1],
		Repo:     parts[2],
		Tag:      parts[3],
		Digest:   parts[4],
	}, nil
}
