package command

import (
	"strings"

	"golang.org/x/mod/semver"
)

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
