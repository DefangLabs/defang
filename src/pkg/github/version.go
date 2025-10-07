package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const latestUrl = "https://api.github.com/repos/DefangLabs/defang/releases/latest"

type githubError struct {
	Message          string
	Status           string
	DocumentationUrl string
}

func GetLatestReleaseTag(ctx context.Context) (string, error) {
	// Anonymous API request to GitHub are rate limited to 60 requests per hour per IP.
	// Check whether the user has set a GitHub token to increase the rate limit. (Copied from the install script.)
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		githubToken = os.Getenv("GH_TOKEN")
	}
	header := http.Header{}
	if githubToken != "" {
		header.Set("Authorization", "Bearer "+githubToken)
	}
	resp, err := http.GetWithHeader(ctx, latestUrl, header)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		term.Debug(resp.Header)
		// The primary rate limit for unauthenticated requests is 60 requests per hour, per IP.
		// The API returns a 403 status code when the rate limit is exceeded.
		githubError := githubError{Message: resp.Status}
		if err := json.NewDecoder(resp.Body).Decode(&githubError); err != nil {
			term.Debugf("Failed to decode GitHub response: %v", err)
		}
		return "", fmt.Errorf("error fetching release info from GitHub: %s", githubError.Message)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}
