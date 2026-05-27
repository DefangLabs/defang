package scaleway

import (
	"context"
	"fmt"
	"os"
)

// APIKeyInfo represents the response from the IAM API key validation endpoint.
type APIKeyInfo struct {
	AccessKey      string `json:"access_key"`
	SecretKey      string `json:"secret_key"`
	DefaultProject string `json:"default_project_id"`
	OrganizationID string `json:"organization_id"`
	Description    string `json:"description"`
	Editable       bool   `json:"editable"`
}

// Authenticate validates the client's credentials by calling the Scaleway IAM API.
func (c *Client) Authenticate(ctx context.Context) (*APIKeyInfo, error) {
	url := fmt.Sprintf("%s/iam/v1alpha1/api-keys/%s", apiBaseURL, c.AccessKey)

	var info APIKeyInfo
	if err := c.doRequestJSON(ctx, "GET", url, nil, &info); err != nil {
		return nil, AnnotateScalewayError(err, "authenticating with Scaleway")
	}

	if info.OrganizationID != "" {
		c.OrganizationID = info.OrganizationID
	}
	return &info, nil
}

// AccountInfo holds the resolved account details for a Scaleway client.
type AccountInfo struct {
	ProjectID      string
	OrganizationID string
	Region         Region
}

// GetAccountInfo returns the project, organization, and region for this client.
func (c *Client) GetAccountInfo() *AccountInfo {
	return &AccountInfo{
		ProjectID:      c.ProjectID,
		OrganizationID: c.OrganizationID,
		Region:         c.Region,
	}
}

// NewClientFromEnv creates a Client from standard Scaleway environment variables.
// Returns an error if required variables are missing.
func NewClientFromEnv() (*Client, error) {
	accessKey := os.Getenv("SCW_ACCESS_KEY")
	secretKey := os.Getenv("SCW_SECRET_KEY")
	projectID := os.Getenv("SCW_DEFAULT_PROJECT_ID")
	region := os.Getenv("SCW_DEFAULT_REGION")

	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("SCW_ACCESS_KEY and SCW_SECRET_KEY must be set (https://www.scaleway.com/en/docs/identity-and-access-management/iam/how-to/create-api-keys/)")
	}
	if projectID == "" {
		return nil, fmt.Errorf("SCW_DEFAULT_PROJECT_ID must be set (https://www.scaleway.com/en/docs/identity-and-access-management/organizations-and-projects/how-to/create-a-project/)")
	}
	if region == "" {
		region = "fr-par"
	}

	client := NewClient(accessKey, secretKey, projectID, region)
	client.OrganizationID = os.Getenv("SCW_DEFAULT_ORGANIZATION_ID")
	return client, nil
}
