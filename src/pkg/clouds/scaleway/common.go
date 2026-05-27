package scaleway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	apiBaseURL = "https://api.scaleway.com"
)

// Region represents a Scaleway region (e.g., "fr-par", "nl-ams", "pl-waw").
type Region = string

// Client provides low-level access to Scaleway APIs using net/http.
type Client struct {
	AccessKey      string
	SecretKey      string
	ProjectID      string
	OrganizationID string
	Region         Region
	HTTPClient     *http.Client
}

// NewClient creates a new Scaleway API client.
func NewClient(accessKey, secretKey, projectID, region string) *Client {
	return &Client{
		AccessKey:  accessKey,
		SecretKey:  secretKey,
		ProjectID:  projectID,
		Region:     region,
		HTTPClient: http.DefaultClient,
	}
}

// DefaultZone returns the default zone for a region (e.g., "fr-par" → "fr-par-1").
func DefaultZone(region Region) string {
	return region + "-1"
}

// doRequest executes an authenticated HTTP request against the Scaleway API.
func (c *Client) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("X-Auth-Token", c.SecretKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.HTTPClient.Do(req)
}

// doRequestJSON executes a request and decodes the JSON response into result.
func (c *Client) doRequestJSON(ctx context.Context, method, url string, body, result any) error {
	resp, err := c.doRequest(ctx, method, url, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseAPIError(resp)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// regionURL returns the base URL for a regional API endpoint.
func (c *Client) regionURL(service, version string) string {
	return fmt.Sprintf("%s/%s/%s/regions/%s", apiBaseURL, service, version, c.Region)
}
