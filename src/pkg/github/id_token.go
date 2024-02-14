package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/defang-io/defang/src/pkg/http"
)

// GitHub OIDC docs: https://docs.github.com/en/actions/deployment/security-hardening-your-deployments/about-security-hardening-with-openid-connect

type actionsIdTokenResponse struct {
	Count int    `json:"count"`
	Value string `json:"value"`
}

func GetIdToken(ctx context.Context) (string, error) {
	requestUrl := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	requestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if requestUrl == "" || requestToken == "" {
		return "", fmt.Errorf("ACTIONS_ID_TOKEN_REQUEST_URL or ACTIONS_ID_TOKEN_REQUEST_TOKEN not set")
	}

	// TODO: append &audience=… to specify the audience
	resp, err := http.GetWithAuth(ctx, requestUrl, "Bearer "+requestToken)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var actionsIdTokenResponse actionsIdTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&actionsIdTokenResponse); err != nil {
		return "", err
	}
	return actionsIdTokenResponse.Value, nil
}
