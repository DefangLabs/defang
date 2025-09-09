package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/golang-jwt/jwt/v5"
)

var (
	selectedTenantName string
	selectedTenantID   string
	autoSelectBySub    bool

	// Returned when multiple tenants share the same name in the userinfo response.
	ErrMultipleTenantMatches = errors.New("multiple tenants match the name")
	// Returned when no tenant matches the provided name in the userinfo response.
	ErrTenantNotFound = errors.New("tenant not found")
	// Returned when no access token is available yet (user not logged in).
	ErrNoAccessToken = errors.New("no access token available; please login first")
)

// SetSelectedTenantName stores the desired tenant name for selection.
func SetSelectedTenantName(name string) {
	selectedTenantName = strings.TrimSpace(name)
	autoSelectBySub = name == ""
}

// SetAutoSelectBySub enables or disables auto-select by JWT sub.
func SetAutoSelectBySub(enabled bool) {
	autoSelectBySub = enabled
}

// subFromJWT extracts the "sub" claim from the given JWT without verification.
func subFromJWT(token string) (string, error) {
	var claims jwt.RegisteredClaims
	_, _, err := new(jwt.Parser).ParseUnverified(token, &claims)
	if err != nil {
		return "", fmt.Errorf("failed to parse access token: %w", err)
	}
	if claims.Subject == "" {
		return "", errors.New("invalid subject (sub) claim in token")
	}
	return claims.Subject, nil
}

// GetSelectedTenantName returns the currently selected tenant name.
func GetSelectedTenantName() string { return selectedTenantName }

// SetSelectedTenantID stores the resolved tenant ID used in Fabric requests.
func SetSelectedTenantID(id string) { selectedTenantID = strings.TrimSpace(id) }

// GetSelectedTenantID returns the currently selected tenant ID.
func GetSelectedTenantID() string { return selectedTenantID }

// issuerFromJWT extracts the "iss" claim from the given JWT without verification.
func issuerFromJWT(token string) (string, error) {
	var claims jwt.RegisteredClaims
	_, _, err := new(jwt.Parser).ParseUnverified(token, &claims)
	if err != nil {
		return "", fmt.Errorf("failed to parse access token: %w", err)
	}
	if claims.Issuer == "" {
		return "", errors.New("invalid issuer (iss) claim in token")
	}
	return claims.Issuer, nil
}

// userinfoTenant represents a tenant entry in the /userinfo payload.
type userinfoTenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// userinfoResponse represents the relevant portion of the /userinfo response.
type userinfoResponse struct {
	AllTenants []userinfoTenant `json:"allTenants"`
}

func invokeUserinfoEndpoint(ctx context.Context, accessToken string) (*userinfoResponse, error) {
	iss, err := issuerFromJWT(accessToken)
	if err != nil {
		return nil, err
	}

	url, _ := url.JoinPath(iss, "userinfo")
	header := http.Header{}
	header.Set("Accept", "application/json")
	header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.GetWithHeader(ctx, url, header)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo request failed: %s", resp.Status)
	}

	var ui userinfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&ui); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo: %w", err)
	}
	return &ui, nil
}

// ResolveAndSetTenantFromToken resolves the tenant ID for the previously set tenant name
// by calling issuer + "/userinfo" with the current access token. On success, it sets the
// global selected tenant ID so subsequent Fabric requests include the header.
func ResolveAndSetTenantFromToken(ctx context.Context, accessToken string) error {
	// If neither a specific name was requested nor auto-select was enabled, do nothing
	if strings.TrimSpace(selectedTenantName) == "" && !autoSelectBySub {
		return nil
	}

	token := strings.TrimSpace(accessToken)
	if token == "" {
		return ErrNoAccessToken
	}

	iss, err := issuerFromJWT(token)
	if err != nil {
		return err
	}

	// If the token is from GitHub Actions, then we do not
	// use the userinfo endpoint to resolve the tenant ID.
	if iss == "https://fabric-prod1.defang.dev" {
		return nil
	}

	ui, err := invokeUserinfoEndpoint(ctx, token)
	if err != nil {
		return err
	}

	if autoSelectBySub {
		sub, err := subFromJWT(token)
		if err != nil {
			return err
		}
		matches := 0
		var id string
		for _, t := range ui.AllTenants {
			if t.ID == sub {
				id = t.ID
				matches++
			}
		}
		switch matches {
		case 0:
			return fmt.Errorf("%w: no tenant with id matching JWT sub", ErrTenantNotFound)
		case 1:
			SetSelectedTenantID(id)
			return nil
		default:
			return fmt.Errorf("%w: multiple tenants with id %q", ErrMultipleTenantMatches, sub)
		}
	} else {
		var (
			id    string
			count int
		)
		for _, t := range ui.AllTenants {
			if t.Name == selectedTenantName {
				id = t.ID
				count++
			}
		}
		switch count {
		case 0:
			return fmt.Errorf("%w: %q", ErrTenantNotFound, selectedTenantName)
		case 1:
			SetSelectedTenantID(id)
			return nil
		default:
			return fmt.Errorf("%w: %q", ErrMultipleTenantMatches, selectedTenantName)
		}
	}
}

// Tenant represents a tenant entry returned by the /userinfo endpoint.
type Tenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListTenantsFromToken calls issuer + "/userinfo" with the provided access token
// and returns the list of tenants available to the user.
func ListTenantsFromToken(ctx context.Context, accessToken string) ([]Tenant, error) {
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return nil, ErrNoAccessToken
	}

	ui, err := invokeUserinfoEndpoint(ctx, token)
	if err != nil {
		return nil, err
	}

	tenants := make([]Tenant, 0, len(ui.AllTenants))
	for _, t := range ui.AllTenants {
		tenants = append(tenants, Tenant{ID: t.ID, Name: t.Name})
	}
	return tenants, nil
}
