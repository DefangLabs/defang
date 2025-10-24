// Package tools: consolidated interfaces for mocking CLI/tool dependencies
package tools

import (
	"context"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
)

// --- Common method sets ---
type connecter interface {
	Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error)
}

type providerFactory interface {
	NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient) cliClient.Provider
}

type projectNameLoader interface {
	LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error)
}
