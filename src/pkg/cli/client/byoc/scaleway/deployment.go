package scaleway

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"slices"

	"github.com/DefangLabs/defang/src/pkg/clouds/scaleway"
	"github.com/DefangLabs/defang/src/pkg/deployment"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/smithy-go"
)

var errMetadataNotFound = deployment.ErrNotFound

func (b *ByocScaleway) readDeployment(ctx context.Context, project, etag string) (*deployment.Metadata, error) {
	// The second key supports DIY backends created before project-scoped paths.
	keys := []string{
		path.Join(".pulumi/stacks", project, b.PulumiStack+".json"),
		path.Join(".pulumi/stacks", b.PulumiStack+".json"),
	}
	for _, key := range keys {
		data, err := scaleway.GetObject(ctx, b.s3Client, b.bucketName(), key)
		if err != nil {
			var apiErr smithy.APIError
			if scaleway.IsNotFound(err) || (errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "NoSuchBucket" || apiErr.ErrorCode() == "NotFound")) {
				continue
			}
			return nil, fmt.Errorf("reading deployment state: %w", err)
		}
		metadata, err := deployment.ReadState(data, "scaleway", project, b.PulumiStack, etag)
		if errors.Is(err, deployment.ErrNotFound) {
			continue
		}
		return metadata, err
	}
	return nil, deployment.ErrNotFound
}

func (b *ByocScaleway) getServicesFromState(ctx context.Context, project string) (*defangv1.GetServicesResponse, error) {
	metadata, err := b.readDeployment(ctx, project, b.cdEtag)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(metadata.Services))
	for name := range metadata.Services {
		names = append(names, name)
	}
	slices.Sort(names)
	response := &defangv1.GetServicesResponse{Project: project}
	for _, name := range names {
		service := metadata.Services[name]
		info := &defangv1.ServiceInfo{
			Service:         &defangv1.Service{Name: name},
			Project:         project,
			Etag:            metadata.Etag,
			Status:          "unknown",
			State:           defangv1.ServiceState_NOT_SPECIFIED,
			HealthcheckPath: service.HealthcheckPath,
		}
		if service.Endpoint != "" {
			info.Endpoints = []string{service.Endpoint}
			if endpoint, err := url.Parse(service.Endpoint); err == nil && endpoint.Host != "" && (endpoint.Scheme == "https" || endpoint.Scheme == "http") {
				info.PublicFqdn = endpoint.Host
			} else if service.Kind != deployment.KindContainer {
				info.PrivateFqdn = service.Endpoint
			}
		}
		if service.Kind == deployment.KindContainer {
			container, err := b.client.GetContainer(ctx, service.ID)
			if err != nil {
				if !scaleway.IsNotFound(err) {
					return nil, err
				}
				info.Status = "not found"
			} else {
				info.Status = container.Status
				switch container.Status {
				case "ready":
					info.State = defangv1.ServiceState_DEPLOYMENT_COMPLETED
				case "error":
					info.State = defangv1.ServiceState_DEPLOYMENT_FAILED
				case "pending", "creating", "deleting", "locked":
					info.State = defangv1.ServiceState_DEPLOYMENT_PENDING
				}
			}
		}
		response.Services = append(response.Services, info)
	}
	return response, nil
}
