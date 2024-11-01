package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func Delete(ctx context.Context, loader client.Loader, client client.FabricClient, provider client.Provider, names ...string) (types.ETag, error) {
	projectName, err := LoadProjectName(ctx, loader, provider)
	if err != nil {
		return "", err
	}

	term.Debug("Deleting service", names)

	if DoDryRun {
		return "", ErrDryRun
	}

	delegateDomain, err := client.GetDelegateSubdomainZone(ctx)
	if err != nil {
		term.Debug("Failed to get delegate domain:", err)
	}

	resp, err := provider.Delete(ctx, &defangv1.DeleteRequest{Project: projectName, Names: names, DelegateDomain: delegateDomain.Zone})
	if err != nil {
		return "", err
	}
	return resp.Etag, nil
}
