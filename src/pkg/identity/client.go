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
	"strings"

	defangHttp "github.com/DefangLabs/defang/src/pkg/http"
	"github.com/go-jose/go-jose/v4"
)

// TenantURL derives the tenant's issuer URL from the apex issuer, e.g.
// https://auth.defang.io + "acme" → https://acme.auth.defang.io. The
// subdomain label is the tenant, per the agent-identity design.
func TenantURL(issuer, tenant string) (string, error) {
	if tenant == "" {
		return "", errors.New("no tenant selected; log in or set DEFANG_WORKSPACE")
	}
	u, err := url.Parse(issuer)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid issuer URL %q", issuer)
	}
	u.Host = tenant + "." + u.Host
	u.Path = ""
	return u.String(), nil
}

// Client talks to one tenant's key registry with a bearer token from the
// OpenAuth issuer (the token saved by `defang login`).
type Client struct {
	tenantURL   string
	accessToken string
}

func NewClient(tenantURL, accessToken string) *Client {
	return &Client{tenantURL: strings.TrimSuffix(tenantURL, "/"), accessToken: accessToken}
}

type RegisterRequest struct {
	ProjectID  string          `json:"project_id"`
	StackID    string          `json:"stack_id"`
	JWK        jose.JSONWebKey `json:"jwk"`
	PopJWT     string          `json:"pop_jwt"`
	TTLSeconds int             `json:"ttl_seconds,omitempty"`
}

// RegisteredKey is a key record as returned by the registry; POST /keys
// returns kid/sub/issuer/exp, GET /keys additionally has project/stack/created.
type RegisteredKey struct {
	Kid       string `json:"kid"`
	Subject   string `json:"sub"`
	Issuer    string `json:"issuer,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	StackID   string `json:"stack_id,omitempty"`
	Created   int64  `json:"created,omitempty"`
	Expires   int64  `json:"exp,omitempty"`
}

func (c *Client) Register(ctx context.Context, req RegisterRequest) (*RegisteredKey, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	header := http.Header{
		"Authorization": []string{"Bearer " + c.accessToken},
		"Content-Type":  []string{"application/json"},
	}
	resp, err := defangHttp.PostWithHeader(ctx, c.tenantURL+"/keys", header, bytes.NewReader(body))
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

func (c *Client) List(ctx context.Context) ([]RegisteredKey, error) {
	resp, err := defangHttp.GetWithAuth(ctx, c.tenantURL+"/keys", "Bearer "+c.accessToken)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var list struct {
		Keys []RegisteredKey `json:"keys"`
	}
	if err := decodeResponse(resp, &list); err != nil {
		return nil, err
	}
	return list.Keys, nil
}

func (c *Client) Revoke(ctx context.Context, kid string) error {
	resp, err := defangHttp.DeleteWithAuth(ctx, c.tenantURL+"/keys/"+url.PathEscape(kid), "Bearer "+c.accessToken)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeResponse(resp, nil)
}

// decodeResponse decodes a 2xx JSON body into out (if non-nil), or surfaces
// the registry's {"error": …} message on failure.
func decodeResponse(resp *http.Response, out any) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var oauthError struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &oauthError) == nil && oauthError.Error != "" {
			return fmt.Errorf("key registry: %s (%s)", oauthError.Error, resp.Status)
		}
		return fmt.Errorf("key registry: unexpected status %s", resp.Status)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("key registry: invalid response: %w", err)
	}
	return nil
}
