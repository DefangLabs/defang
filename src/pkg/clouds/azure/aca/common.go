package aca

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers"
	cloudazure "github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

const apiVersion = "2023-05-01"

type ContainerApp struct {
	cloudazure.Azure
	ResourceGroup string
}

func (c *ContainerApp) newContainerAppsClient() (*armappcontainers.ContainerAppsClient, error) {
	cred, err := c.NewCreds()
	if err != nil {
		return nil, err
	}
	return armappcontainers.NewContainerAppsClient(c.SubscriptionID, cred, nil)
}

func (c *ContainerApp) newReplicasClient() (*armappcontainers.ContainerAppsRevisionReplicasClient, error) {
	cred, err := c.NewCreds()
	if err != nil {
		return nil, err
	}
	return armappcontainers.NewContainerAppsRevisionReplicasClient(c.SubscriptionID, cred, nil)
}

// armToken returns a Bearer token scoped to the Azure management endpoint.
func (c *ContainerApp) armToken(ctx context.Context) (string, error) {
	cred, err := c.NewCreds()
	if err != nil {
		return "", err
	}
	tok, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return "", err
	}
	return tok.Token, nil
}

// getAuthToken fetches a short-lived token for the Container Apps log-stream endpoint.
// This operation is not yet exposed in the ARM Go SDK, so we call the REST API directly.
func (c *ContainerApp) getAuthToken(ctx context.Context, appName string) (string, error) {
	armTok, err := c.armToken(ctx)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/containerApps/%s/getAuthToken?api-version=%s",
		c.SubscriptionID, c.ResourceGroup, appName, apiVersion,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+armTok)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("getAuthToken: HTTP %s", resp.Status)
	}

	var result struct {
		Properties struct {
			Token string `json:"token"`
		} `json:"properties"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("getAuthToken: decode response: %w", err)
	}
	return result.Properties.Token, nil
}

// getEventStreamBase returns the host portion of the container app's eventStreamEndpoint
// (everything before "/subscriptions/"). This is not in SDK v1.1.0, so we call the REST API directly.
func (c *ContainerApp) getEventStreamBase(ctx context.Context, appName string) (string, error) {
	armTok, err := c.armToken(ctx)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/containerApps/%s?api-version=%s",
		c.SubscriptionID, c.ResourceGroup, appName, apiVersion,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+armTok)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("getContainerApp: HTTP %s", resp.Status)
	}

	var result struct {
		Properties struct {
			EventStreamEndpoint string `json:"eventStreamEndpoint"`
		} `json:"properties"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("getContainerApp: decode response: %w", err)
	}
	endpoint := result.Properties.EventStreamEndpoint
	idx := strings.Index(endpoint, "/subscriptions/")
	if idx < 0 {
		return "", fmt.Errorf("unexpected eventStreamEndpoint format: %q", endpoint)
	}
	return endpoint[:idx], nil
}

// ResolveLogTarget resolves the latest active revision, first replica, and first container
// name for the given app. Any of the return values that were already provided as non-empty
// strings are passed through unchanged.
func (c *ContainerApp) ResolveLogTarget(ctx context.Context, appName, revision, replica, container string) (string, string, string, error) {
	if revision == "" {
		appsClient, err := c.newContainerAppsClient()
		if err != nil {
			return "", "", "", err
		}
		app, err := appsClient.Get(ctx, c.ResourceGroup, appName, nil)
		if err != nil {
			return "", "", "", fmt.Errorf("get container app: %w", err)
		}
		if app.Properties == nil || app.Properties.LatestRevisionName == nil {
			return "", "", "", fmt.Errorf("container app %q has no active revision", appName)
		}
		revision = *app.Properties.LatestRevisionName

		// Opportunistically pick the container name from the app template.
		if container == "" && app.Properties.Template != nil && len(app.Properties.Template.Containers) > 0 {
			container = *app.Properties.Template.Containers[0].Name
		}
	}

	if replica == "" {
		replicasClient, err := c.newReplicasClient()
		if err != nil {
			return "", "", "", err
		}
		list, err := replicasClient.ListReplicas(ctx, c.ResourceGroup, appName, revision, nil)
		if err != nil {
			return "", "", "", fmt.Errorf("list replicas: %w", err)
		}
		if len(list.Value) == 0 {
			return "", "", "", fmt.Errorf("no replicas found for revision %q", revision)
		}
		replica = *list.Value[0].Name

		// Opportunistically pick the container from the replica if still unset.
		if container == "" && len(list.Value[0].Properties.Containers) > 0 {
			container = *list.Value[0].Properties.Containers[0].Name
		}
	}

	if container == "" {
		return "", "", "", fmt.Errorf("could not determine container name for app %q", appName)
	}

	return revision, replica, container, nil
}
