package scaleway

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/types"
)

type ByocScaleway struct {
	*byoc.ByocBaseClient
}

func NewByocProvider(ctx context.Context, tenantName types.TenantName) *ByocScaleway {
	b := &ByocScaleway{}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, tenantName, b)
	return b
}

var _ client.Provider = (*ByocScaleway)(nil)
