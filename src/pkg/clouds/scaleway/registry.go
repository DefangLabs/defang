package scaleway

import (
	"context"
	"fmt"
	"net/url"
)

// RegistryNamespace represents a Scaleway Container Registry namespace.
type RegistryNamespace struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ProjectID      string `json:"project_id"`
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region"`
	Status         string `json:"status"`
	IsPublic       bool   `json:"is_public"`
	OrganizationID string `json:"organization_id"`
	CreatedAt      string `json:"created_at"`
}

// RegistryImage represents an image in a Container Registry namespace.
type RegistryImage struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	NamespaceID string   `json:"namespace_id"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags"`
	Size        int64    `json:"size"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type listRegistryNamespacesResponse struct {
	Namespaces []RegistryNamespace `json:"namespaces"`
	TotalCount int                 `json:"total_count"`
}

type listRegistryImagesResponse struct {
	Images     []RegistryImage `json:"images"`
	TotalCount int             `json:"total_count"`
}

// EnsureRegistryNamespaceExists creates a registry namespace if it does not already exist.
// Returns the existing or newly created namespace.
func (c *Client) EnsureRegistryNamespaceExists(ctx context.Context, name, projectID, region string) (*RegistryNamespace, error) {
	if projectID == "" {
		projectID = c.ProjectID
	}
	if region == "" {
		region = c.Region
	}

	baseURL := fmt.Sprintf("%s/registry/v1/regions/%s", apiBaseURL, region)

	// List namespaces to check if one exists with this name
	listURL := fmt.Sprintf("%s/namespaces?project_id=%s&name=%s", baseURL, url.QueryEscape(projectID), url.QueryEscape(name))
	var listResp listRegistryNamespacesResponse
	if err := c.doRequestJSON(ctx, "GET", listURL, nil, &listResp); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("listing registry namespaces for %q", name))
	}
	for i := range listResp.Namespaces {
		if listResp.Namespaces[i].Name == name {
			return &listResp.Namespaces[i], nil
		}
	}

	// Create the namespace
	createURL := fmt.Sprintf("%s/namespaces", baseURL)
	body := map[string]any{
		"name":       name,
		"project_id": projectID,
		"is_public":  false,
	}
	var ns RegistryNamespace
	if err := c.doRequestJSON(ctx, "POST", createURL, body, &ns); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("creating registry namespace %q", name))
	}
	return &ns, nil
}

// GetRegistryEndpoint returns the registry endpoint for a namespace in a region.
// Format: rg.{region}.scw.cloud/{namespace}
func GetRegistryEndpoint(region, namespace string) string {
	return fmt.Sprintf("rg.%s.scw.cloud/%s", region, namespace)
}

// ListImages lists images in a Container Registry namespace.
func (c *Client) ListImages(ctx context.Context, namespaceID string) ([]RegistryImage, error) {
	url := fmt.Sprintf("%s/registry/v1/regions/%s/images?namespace_id=%s", apiBaseURL, c.Region, namespaceID)
	var resp listRegistryImagesResponse
	if err := c.doRequestJSON(ctx, "GET", url, nil, &resp); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("listing images for namespace %q", namespaceID))
	}
	return resp.Images, nil
}

// ListRegistryNamespaces lists registry namespaces in a project, optionally filtered by name.
func (c *Client) ListRegistryNamespaces(ctx context.Context, projectID, name string) ([]RegistryNamespace, error) {
	if projectID == "" {
		projectID = c.ProjectID
	}
	apiURL := fmt.Sprintf("%s/registry/v1/regions/%s/namespaces?project_id=%s", apiBaseURL, c.Region, url.QueryEscape(projectID))
	if name != "" {
		apiURL += "&name=" + url.QueryEscape(name)
	}
	var resp listRegistryNamespacesResponse
	if err := c.doRequestJSON(ctx, "GET", apiURL, nil, &resp); err != nil {
		return nil, AnnotateScalewayError(err, "listing registry namespaces")
	}
	return resp.Namespaces, nil
}

func (c *Client) DeleteImage(ctx context.Context, imageID string) error {
	url := fmt.Sprintf("%s/registry/v1/regions/%s/images/%s", apiBaseURL, c.Region, imageID)
	if err := c.doRequestJSON(ctx, "DELETE", url, nil, nil); err != nil {
		return AnnotateScalewayError(err, fmt.Sprintf("deleting registry image %q", imageID))
	}
	return nil
}

func (c *Client) DeleteRegistryNamespace(ctx context.Context, namespaceID string) error {
	url := fmt.Sprintf("%s/registry/v1/regions/%s/namespaces/%s", apiBaseURL, c.Region, namespaceID)
	if err := c.doRequestJSON(ctx, "DELETE", url, nil, nil); err != nil {
		return AnnotateScalewayError(err, fmt.Sprintf("deleting registry namespace %q", namespaceID))
	}
	return nil
}
