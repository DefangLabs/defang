package client

import (
	"context"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func CanIUseProvider(ctx context.Context, client FabricClient, provider Provider, projectName, stack string, serviceCount int) error {
	info, err := provider.AccountInfo(ctx)
	if err != nil {
		return err
	}

	canUseReq := defangv1.CanIUseRequest{
		Project:           projectName,
		Provider:          info.Provider.Value(),
		ProviderAccountId: info.AccountID,
		Region:            info.Region,
		ServiceCount:      int32(serviceCount), // #nosec G115 - service count will not overflow int32
		Stack:             stack,
	}

	resp, err := client.CanIUse(ctx, &canUseReq)
	if err != nil {
		return err
	}
	provider.SetCanIUseConfig(resp)
	return nil
}
