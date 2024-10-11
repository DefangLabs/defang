package appPlatform

import (
	"fmt"
	"regexp"
)

// Copied from defang-mvp/pulumi/shared/utils.ts
var IMG_RE = regexp.MustCompile(`^((?:(.+?)\/)?(.+?)(?:@(sha256:[0-9a-f]{64})|:(\w[\w.-]{0,127}))?)$`) // FIXME: avoid unbounded "+" in the regex

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
		Registry: parts[2],
		Repo:     parts[3],
		Digest:   parts[4],
		Tag:      parts[5],
	}, nil
}
