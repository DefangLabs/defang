package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
)

type BuildTag struct {
	Stack   string
	Project string
	Service string
	Etag    string
}

func (bt BuildTag) String() string {
	if bt.Stack == "" {
		return fmt.Sprintf("%s_%s_%s", bt.Project, bt.Service, bt.Etag)
	} else {
		return fmt.Sprintf("%s_%s_%s_%s", bt.Stack, bt.Project, bt.Service, bt.Etag)
	}
}

func (bt *BuildTag) Parse(tag string) error {
	parts := strings.Split(tag, "_")
	if len(parts) < 3 || len(parts) > 4 {
		return fmt.Errorf("invalid cloudbuild build tags value: %q", tag)
	}

	if len(parts) == 3 { // Backward compatibility
		bt.Stack = ""
		bt.Project = parts[0]
		bt.Service = parts[1]
		bt.Etag = parts[2]
	} else {
		bt.Stack = parts[0]
		bt.Project = parts[1]
		bt.Service = parts[2]
		bt.Etag = parts[3]
	}
	return nil
}

func (gcp Gcp) GetBuildInfo(ctx context.Context, buildId string) (*BuildTag, error) {
	client, err := cloudbuild.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloudbuild client: %w", err)
	}
	defer client.Close()
	req := &cloudbuildpb.GetBuildRequest{
		ProjectId: gcp.ProjectId,
		Id:        buildId,
	}
	build, err := client.GetBuild(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get build: %w", err)
	}
	if build == nil {
		return nil, errors.New("build not found")
	}
	var bt BuildTag
	for _, tag := range build.Tags {
		if err := bt.Parse(tag); err == nil {
			return &bt, nil
		}
	}
	return nil, fmt.Errorf("cannot find build tag containing build info: %v", build.Tags)
}
