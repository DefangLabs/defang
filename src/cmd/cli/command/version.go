package command

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/mod/semver"
)

var httpClient = http.DefaultClient

func isNewer(current, comparand string) bool {
	version, ok := normalizeVersion(current)
	if !ok {
		return false // development versions are always considered "latest"
	}
	return semver.Compare(version, comparand) < 0
}

func isGitRef(maybeVersion string) bool {
	return len(maybeVersion) >= 7 && !strings.Contains(maybeVersion, ".")
}

func normalizeVersion(maybeVersion string) (string, bool) {
	version := "v" + maybeVersion
	if semver.IsValid(version) && !isGitRef(maybeVersion) {
		return version, true
	}
	return maybeVersion, false // leave as is
}

func GetCurrentVersion() string {
	version, _ := normalizeVersion(RootCmd.Version)
	return version
}

func GetLatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/DefangLabs/defang/releases/latest", nil)
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
	return release.TagName, nil
}
