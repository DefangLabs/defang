package command

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"golang.org/x/mod/semver"
)

var httpClient = http.DefaultClient

func GetCurrentVersion() string {
	version := RootCmd.Version
	if v := semver.Canonical("v" + version); v != "" {
		return v
	}
	return version
}

func GetLatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/defang-io/defang/releases/latest", nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// The primary rate limit for unauthenticated requests is 60 requests per hour, per IP.
		return "", errors.New(resp.Status)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return semver.Canonical(release.TagName), nil
}
