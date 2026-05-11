package scaleway

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Secret represents a Scaleway Secret Manager secret.
type Secret struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// SecretVersion represents a version of a secret.
type SecretVersion struct {
	Revision  int    `json:"revision"`
	SecretID  string `json:"secret_id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// SecretVersionAccess contains the actual secret data.
type SecretVersionAccess struct {
	SecretID string `json:"secret_id"`
	Revision int    `json:"revision"`
	Data     string `json:"data"` // base64-encoded
}

type listSecretsResponse struct {
	Secrets    []Secret `json:"secrets"`
	TotalCount int      `json:"total_count"`
}

type listSecretVersionsResponse struct {
	Versions   []SecretVersion `json:"versions"`
	TotalCount int             `json:"total_count"`
}

// CreateSecret creates a new secret in Scaleway Secret Manager.
// Returns the secret and a nil error on success. If the secret already exists
// (Scaleway returns 400 with "cannot have same secret name"), the error wraps
// an APIError with status 409 so callers can use IsConflict.
func (c *Client) CreateSecret(ctx context.Context, name, projectID string) (*Secret, error) {
	if projectID == "" {
		projectID = c.ProjectID
	}
	url := c.regionURL("secret-manager", "v1beta1") + "/secrets"
	body := map[string]string{
		"project_id": projectID,
		"name":       name,
	}
	var secret Secret
	if err := c.doRequestJSON(ctx, "POST", url, body, &secret); err != nil {
		// Scaleway returns 400 (not 409) for duplicate secret names; normalize to 409
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 400 && strings.Contains(apiErr.RawBody, "same secret name") {
			apiErr.StatusCode = 409
			return nil, apiErr
		}
		return nil, AnnotateScalewayError(err, fmt.Sprintf("creating secret %q", name))
	}
	return &secret, nil
}

// CreateSecretVersion adds a new version with the given data to a secret.
// The data is base64-encoded before sending.
func (c *Client) CreateSecretVersion(ctx context.Context, secretID string, data []byte) (*SecretVersion, error) {
	url := c.regionURL("secret-manager", "v1beta1") + fmt.Sprintf("/secrets/%s/versions", secretID)
	body := map[string]string{
		"data": base64.StdEncoding.EncodeToString(data),
	}
	var version SecretVersion
	if err := c.doRequestJSON(ctx, "POST", url, body, &version); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("creating secret version for %q", secretID))
	}
	return &version, nil
}

// GetSecretVersion retrieves a specific version of a secret.
// Use revision "latest" for the most recent version, or a numeric string.
func (c *Client) GetSecretVersion(ctx context.Context, secretID, revision string) (*SecretVersionAccess, error) {
	url := c.regionURL("secret-manager", "v1beta1") + fmt.Sprintf("/secrets/%s/versions/%s/access", secretID, revision)
	var access SecretVersionAccess
	if err := c.doRequestJSON(ctx, "GET", url, nil, &access); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("getting secret version %s/%s", secretID, revision))
	}
	return &access, nil
}

// ListSecrets lists secrets in a project, optionally filtered by name prefix.
// Note: The Scaleway API's name parameter does exact matching, so we perform
// client-side prefix filtering when a prefix is provided.
func (c *Client) ListSecrets(ctx context.Context, projectID, prefix string) ([]Secret, error) {
	if projectID == "" {
		projectID = c.ProjectID
	}
	endpoint := c.regionURL("secret-manager", "v1beta1") + "/secrets"
	params := url.Values{
		"project_id": {projectID},
	}
	// Try exact match first via the API; if prefix is provided we'll filter client-side
	if prefix != "" {
		params.Set("name", prefix)
	}
	fullURL := endpoint + "?" + params.Encode()

	var resp listSecretsResponse
	if err := c.doRequestJSON(ctx, "GET", fullURL, nil, &resp); err != nil {
		return nil, AnnotateScalewayError(err, "listing secrets")
	}

	// If exact match returned results or no prefix was given, return as-is
	if len(resp.Secrets) > 0 || prefix == "" {
		return resp.Secrets, nil
	}

	// Scaleway API does exact matching, not prefix matching.
	// Fall back to listing all secrets and filtering client-side by prefix.
	allURL := endpoint + "?" + url.Values{"project_id": {projectID}}.Encode()
	var allResp listSecretsResponse
	if err := c.doRequestJSON(ctx, "GET", allURL, nil, &allResp); err != nil {
		return nil, AnnotateScalewayError(err, "listing secrets")
	}

	filtered := make([]Secret, 0, len(allResp.Secrets))
	for _, s := range allResp.Secrets {
		if strings.HasPrefix(s.Name, prefix) {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

// DeleteSecret permanently deletes a secret and all its versions.
func (c *Client) DeleteSecret(ctx context.Context, secretID string) error {
	url := c.regionURL("secret-manager", "v1beta1") + fmt.Sprintf("/secrets/%s", secretID)
	if err := c.doRequestJSON(ctx, "DELETE", url, nil, nil); err != nil {
		return AnnotateScalewayError(err, fmt.Sprintf("deleting secret %q", secretID))
	}
	return nil
}
