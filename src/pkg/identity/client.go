package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	defangHttp "github.com/DefangLabs/defang/src/pkg/http"
	"github.com/go-jose/go-jose/v4"
)

// DefaultDomain is where tenant issuers live: tenant "acme" is the issuer
// https://acme.id.defang.io.
const DefaultDomain = "id.defang.io"

// Same rule as the server: one DNS label of 3 to 40 characters, no "--".
var tenantName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$`)

// IssuerURL returns the OIDC issuer for a tenant. This is the value a cloud
// trust policy names, so it is built in exactly one place.
func IssuerURL(domain, tenant string) (string, error) {
	if tenant == "" {
		return "", errors.New("no identity tenant selected; pass --tenant or set DEFANG_ID_TENANT")
	}
	if !tenantName.MatchString(tenant) || strings.Contains(tenant, "--") {
		return "", fmt.Errorf("invalid identity tenant %q: use 3 to 40 lowercase letters, digits or single hyphens", tenant)
	}
	if domain == "" {
		domain = DefaultDomain
	}
	return "https://" + tenant + "." + domain, nil
}

// Client talks to id.defang.io with an owner bearer token (the token saved by
// `defang login`).
type Client struct {
	domain      string
	accessToken string
}

func NewClient(domain, accessToken string) *Client {
	if domain == "" {
		domain = DefaultDomain
	}
	return &Client{domain: domain, accessToken: accessToken}
}

// baseURL lets tests point the client at an httptest server.
var baseURL = func(host string) string { return "https://" + host }

type Tenant struct {
	Name   string `json:"name"`
	Issuer string `json:"issuer"`
}

// ClaimTenant claims a tenant name for the caller. Claiming a name the caller
// already owns succeeds again; a name owned by somebody else fails.
func (c *Client) ClaimTenant(ctx context.Context, name string) (*Tenant, error) {
	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return nil, err
	}
	resp, err := defangHttp.PostWithHeader(ctx, baseURL(c.domain)+"/tenants", c.jsonHeader(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var tenant Tenant
	if err := decodeResponse(resp, &tenant); err != nil {
		return nil, err
	}
	return &tenant, nil
}

type RegisterRequest struct {
	JWK        jose.JSONWebKey `json:"jwk"`
	Proof      string          `json:"proof"`
	Label      string          `json:"label,omitempty"`
	TTLSeconds int             `json:"ttl_seconds,omitempty"`
}

type RegisteredKey struct {
	Kid    string `json:"kid"`
	Issuer string `json:"issuer"`
}

// ListedKey is a key as GET /keys reports it, including revoked and expired
// ones; only Active keys are in the published JWKS.
type ListedKey struct {
	Kid       string  `json:"kid"`
	Alg       string  `json:"alg"`
	Label     *string `json:"label"`
	Active    bool    `json:"active"`
	CreatedAt string  `json:"created_at"`
	ExpiresAt *string `json:"expires_at"`
	RevokedAt *string `json:"revoked_at"`
}

func (c *Client) tenantURL(tenant string) string {
	return baseURL(tenant + "." + c.domain)
}

func (c *Client) jsonHeader() http.Header {
	return http.Header{
		"Authorization": []string{"Bearer " + c.accessToken},
		"Content-Type":  []string{"application/json"},
	}
}

func (c *Client) Register(ctx context.Context, tenant string, req RegisterRequest) (*RegisteredKey, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	resp, err := defangHttp.PostWithHeader(ctx, c.tenantURL(tenant)+"/keys", c.jsonHeader(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var registered RegisteredKey
	if err := decodeResponse(resp, &registered); err != nil {
		return nil, err
	}
	return &registered, nil
}

func (c *Client) List(ctx context.Context, tenant string) ([]ListedKey, error) {
	resp, err := defangHttp.GetWithAuth(ctx, c.tenantURL(tenant)+"/keys", "Bearer "+c.accessToken)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var list struct {
		Keys []ListedKey `json:"keys"`
	}
	if err := decodeResponse(resp, &list); err != nil {
		return nil, err
	}
	return list.Keys, nil
}

func (c *Client) Revoke(ctx context.Context, tenant, kid string) error {
	resp, err := defangHttp.DeleteWithAuth(ctx, c.tenantURL(tenant)+"/keys/"+url.PathEscape(kid), "Bearer "+c.accessToken)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeResponse(resp, nil)
}

// decodeResponse decodes a 2xx JSON body into out (if non-nil), or surfaces
// the service's {"error", "message"} on failure.
func decodeResponse(resp *http.Response, out any) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiError struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &apiError) == nil && apiError.Error != "" {
			if resp.StatusCode == http.StatusNotFound && apiError.Error == "not_found" {
				return fmt.Errorf("identity: not found (%s); the tenant does not exist, you do not own it, or the key is unknown", resp.Status)
			}
			if apiError.Message != "" {
				return fmt.Errorf("identity: %s: %s (%s)", apiError.Error, apiError.Message, resp.Status)
			}
			return fmt.Errorf("identity: %s (%s)", apiError.Error, resp.Status)
		}
		return fmt.Errorf("identity: unexpected status %s", resp.Status)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("identity: invalid response: %w", err)
	}
	return nil
}
