package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	"golang.org/x/mod/semver"
)

var version = "development" // overwritten by build script -ldflags "-X main.version=..." and GoReleaser

func GetCurrentVersion() string {
	if v := semver.Canonical("v" + version); v != "" {
		return v
	}
	return version
}

func GetLatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/defang-io/defang/tags", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var tags []struct {
		Name string `json:"name"`
	}
	err = json.NewDecoder(resp.Body).Decode(&tags)
	if err != nil || len(tags) == 0 {
		return "", err
	}
	sort.Slice(tags, func(i, j int) bool {
		return semver.Compare(tags[i].Name, tags[j].Name) > 0
	})
	return semver.Canonical(tags[0].Name), nil
}
