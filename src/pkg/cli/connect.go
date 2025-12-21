package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/do"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/gcp"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/golang-jwt/jwt/v5"
)

func Connect(ctx context.Context, addr string) (*client.GrpcClient, error) {
	return ConnectWithTenant(ctx, addr, types.TenantUnset)
}

// ConnectWithTenant builds a client carrying the requested tenant (name or ID),
// falling back to the token subject when unset so the server can resolve the personal tenant.
func ConnectWithTenant(ctx context.Context, addr string, requestedTenant types.TenantNameOrID) (*client.GrpcClient, error) {
	host := client.NormalizeHost(addr)
	accessToken := client.GetExistingToken(host)
	tokenTenant := TenantFromToken(accessToken)
	effectiveTenant := requestedTenant
	if !effectiveTenant.IsSet() {
		effectiveTenant = tokenTenant
	}

	// Carry all tenant sources so we can emit the requested value (or token fallback) consistently.
	term.Debug("Using tenant", effectiveTenant, "for cluster", host)
	grpcClient := client.NewGrpcClient(host, accessToken, client.TenantContext{
		RequestedTenant: requestedTenant,
		TokenTenant:     tokenTenant,
	})
	track.Tracker = grpcClient // Update track client

	if _, err := grpcClient.WhoAmI(ctx); err != nil {
		term.Debug("Unable to validate tenant with server:", err)
		return grpcClient, err
	}
	return grpcClient, nil
}

// TenantFromToken extracts the subject (tenant id) from an access token without verification.
func TenantFromToken(accessToken string) types.TenantNameOrID {
	if accessToken == "" {
		return types.TenantUnset
	}
	var claims jwt.RegisteredClaims
	if _, _, err := jwt.NewParser().ParseUnverified(accessToken, &claims); err != nil {
		return types.TenantUnset
	}
	if claims.Subject == "" {
		return types.TenantUnset
	}
	return types.TenantNameOrID(claims.Subject)
}

func NewProvider(ctx context.Context, providerID client.ProviderID, fabricClient client.FabricClient, stack string) client.Provider {
	var provider client.Provider
	term.Debugf("Creating %s provider", providerID)
	switch providerID {
	case client.ProviderAWS:
		provider = aws.NewByocProvider(ctx, fabricClient.GetTenantName(), stack)
	case client.ProviderDO:
		provider = do.NewByocProvider(ctx, fabricClient.GetTenantName(), stack)
	case client.ProviderGCP:
		provider = gcp.NewByocProvider(ctx, fabricClient.GetTenantName(), stack)
	default:
		provider = &client.PlaygroundProvider{FabricClient: fabricClient}
	}
	return provider
}
