package cli

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/globals"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func Delete(ctx context.Context, projectName string, c client.FabricClient, provider client.Provider, names ...string) (types.ETag, error) {
	term.Debug("Deleting service", names)

	if globals.Config.DoDryRun {
		return "", globals.ErrDryRun
	}

	delegateDomain, err := c.GetDelegateSubdomainZone(ctx, &defangv1.GetDelegateSubdomainZoneRequest{}) // TODO: pass projectName
	if err != nil {
		term.Debug("GetDelegateSubdomainZone failed:", err)
		return "", errors.New("failed to get delegate domain")
	}

	resp, err := provider.Delete(ctx, &defangv1.DeleteRequest{Project: projectName, Names: names, DelegateDomain: delegateDomain.Zone})
	if err != nil {
		return "", err
	}
	return resp.Etag, nil
}
