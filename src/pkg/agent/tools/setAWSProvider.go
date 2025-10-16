package tools

import (
	"context"
	"errors"
	"fmt"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/actions"
)

type SetAWSProviderParams struct {
	AccessKeyId     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	Region          string `json:"region"`
}

// HandleSetAWSProvider handles the set AWS provider MCP tool request
func HandleSetAWSProvider(ctx context.Context, params SetAWSProviderParams, providerId *cliClient.ProviderID, cluster string) (string, error) {
	if params.AccessKeyId == "" {
		return "", errors.New("AWS access key Id cannot be empty")
	}

	if params.SecretAccessKey == "" {
		return "", errors.New("AWS secret access key cannot be empty")
	}

	if params.Region == "" {
		return "", errors.New("AWS region cannot be empty")
	}

	if err := actions.SetAWSByocProvider(ctx, providerId, cluster, params.AccessKeyId, params.SecretAccessKey, params.Region); err != nil {
		return "", fmt.Errorf("Failed to set AWS provider: %w", err)
	}

	return fmt.Sprintf("Successfully set the provider %q", *providerId), nil
}
