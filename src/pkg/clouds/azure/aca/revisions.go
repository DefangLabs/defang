package aca

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	armappcontainersv3 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
)

// RevisionState captures the three orthogonal lifecycle properties of an
// ACA revision. NotFound is true when the revision does not (yet) exist;
// callers usually want to treat that as "still pending" rather than an error.
type RevisionState struct {
	NotFound          bool
	ProvisioningState armappcontainersv3.RevisionProvisioningState
	RunningState      armappcontainersv3.RevisionRunningState
	HealthState       armappcontainersv3.RevisionHealthState
	ProvisioningError string
}

func (c *ContainerApp) newRevisionsClient() (*armappcontainersv3.ContainerAppsRevisionsClient, error) {
	cred, err := c.NewCreds()
	if err != nil {
		return nil, err
	}
	return armappcontainersv3.NewContainerAppsRevisionsClient(c.SubscriptionID, cred, nil)
}

// GetRevisionState returns the current lifecycle state of a revision. A 404
// response is mapped to RevisionState{NotFound: true} rather than an error
// because the revision is created mid-deploy by Pulumi.
func (c *ContainerApp) GetRevisionState(ctx context.Context, appName, revisionName string) (*RevisionState, error) {
	client, err := c.newRevisionsClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.GetRevision(ctx, c.ResourceGroup, appName, revisionName, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && (respErr.StatusCode == 404 || respErr.ErrorCode == "ResourceNotFound" || respErr.ErrorCode == "RevisionNotFound" || respErr.ErrorCode == "ResourceGroupNotFound") {
			return &RevisionState{NotFound: true}, nil
		}
		return nil, fmt.Errorf("get revision %q: %w", revisionName, err)
	}
	state := &RevisionState{}
	if resp.Properties == nil {
		return state, nil
	}
	if resp.Properties.ProvisioningState != nil {
		state.ProvisioningState = *resp.Properties.ProvisioningState
	}
	if resp.Properties.RunningState != nil {
		state.RunningState = *resp.Properties.RunningState
	}
	if resp.Properties.HealthState != nil {
		state.HealthState = *resp.Properties.HealthState
	}
	if resp.Properties.ProvisioningError != nil {
		state.ProvisioningError = *resp.Properties.ProvisioningError
	}
	return state, nil
}
