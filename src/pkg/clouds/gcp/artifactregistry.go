package gcp

import (
	"context"
	"fmt"
	"strings"

	artifactregistry "cloud.google.com/go/artifactregistry/apiv1"
	"cloud.google.com/go/artifactregistry/apiv1/artifactregistrypb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (gcp Gcp) EnsureArtifactRegistryExists(ctx context.Context, repoName string) (string, error) {
	client, err := artifactregistry.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("artifactregistry.NewClient: %w", err)
	}

	parent := fmt.Sprintf("projects/%s/locations/%s", gcp.ProjectId, gcp.Region)
	fullRepoName := fmt.Sprintf("%s/repositories/%s", parent, repoName)
	if resp, err := client.GetRepository(ctx, &artifactregistrypb.GetRepositoryRequest{Name: fullRepoName}); err != nil {
		if IsNotFound(err) {
			return "", fmt.Errorf("artifactregistry.GetRepository: %w", err)
		}
	} else if resp != nil {
		return resp.Name, nil
	}

	req := &artifactregistrypb.CreateRepositoryRequest{
		Parent:       parent,
		RepositoryId: repoName,
		Repository: &artifactregistrypb.Repository{
			Format:      artifactregistrypb.Repository_DOCKER,
			Description: "Automatically created artifact registry",
		},
	}

	op, err := client.CreateRepository(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create artifact registry: %w", err)
	}
	resp, err := op.Wait(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to wait for artifact registry creation: %w", err)
	}

	return resp.Name, nil
}

func IsNotFound(err error) bool {
	if grpcErr, ok := status.FromError(err); ok {
		if grpcErr.Code() == codes.NotFound {
			return true
		}
		if grpcErr.Code() == codes.Unknown && strings.HasSuffix(grpcErr.Message(), "notFound") {
			return true
		}
	}
	return false
}
