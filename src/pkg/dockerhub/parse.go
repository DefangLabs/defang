package dockerhub

import (
	"fmt"
	"regexp"
)

// Copied from defang-mvp/pulumi/shared/utils.ts
var IMG_RE = regexp.MustCompile(`^((?:((?:[0-9a-z](?:[0-9a-z-]{0,61}[0-9a-z])?\.)+[a-z]{2,63})\/)?(.{1,127}?))(?::(\w[\w.-]{0,127}))?(?:@(sha256:[0-9a-f]{64}))?$`)

type Image struct {
	Image    string
	Registry string
	Repo     string
	Tag      string
	Digest   string
}

func (i Image) String() string {
	result := ""
	if i.Registry != "" {
		result += i.Registry + "/"
	}
	result += i.Repo
	if i.Tag != "" {
		result += ":" + i.Tag
	}
	if i.Digest != "" {
		result += "@" + i.Digest
	}
	return result
}

func (i Image) GoString() string {
	return fmt.Sprintf("Image: %q, Registry: %q, Repo: %q, Tag: %q, Digest: %q", i.Image, i.Registry, i.Repo, i.Tag, i.Digest)
}

func (i Image) FullImage() string {
	result := ""
	if i.Registry != "" {
		result += i.Registry + "/"
	}
	result += i.Repo
	if i.Tag != "" {
		result += ":" + i.Tag
	}
	if i.Digest != "" {
		result += "@" + i.Digest
	}
	return result
}

func ParseImage(image string) (*Image, error) {
	parts := IMG_RE.FindStringSubmatch(image)
	if parts == nil {
		return nil, fmt.Errorf("invalid image: %s", image)
	}
	return &Image{
		Image:    parts[1],
		Registry: parts[2],
		Repo:     parts[3],
		Tag:      parts[4],
		Digest:   parts[5],
	}, nil
}
