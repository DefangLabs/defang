package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/DefangLabs/defang/src/pkg/http"
)

// GitHub OIDC docs: https://docs.github.com/en/actions/deployment/security-hardening-your-deployments/about-security-hardening-with-openid-connect

type actionsIdTokenResponse struct {
	Count int    `json:"count"` // unused
	Value string `json:"value"`
}

func GetIdToken(ctx context.Context, audience string) (string, error) {
	requestUrl := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	requestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if requestUrl == "" || requestToken == "" {
		return "", errors.New("ACTIONS_ID_TOKEN_REQUEST_URL or ACTIONS_ID_TOKEN_REQUEST_TOKEN not set")
	}

	parsedUrl, err := url.Parse(requestUrl)
	if err != nil {
		return "", fmt.Errorf("invalid ACTIONS_ID_TOKEN_REQUEST_URL: %w", err)
	}
	if audience != "" {
		// Add audience query param to any existing query params
		query := parsedUrl.Query()
		query.Set("audience", audience)
		parsedUrl.RawQuery = query.Encode()
	}
	resp, err := http.GetWithAuth(ctx, parsedUrl.String(), "Bearer "+requestToken)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var actionsIdTokenResponse actionsIdTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&actionsIdTokenResponse); err != nil {
		return "", err
	}
	return actionsIdTokenResponse.Value, nil
}
