// Package contract contains Scaleway conventions shared by CD and the CLI.
// It has no dependencies on cloud SDKs or provisioning implementations.
package contract

import "strings"

// ConfigPrefix preserves the names of existing configs, including stack paths.
func ConfigPrefix(prefix, project, stack string) string {
	return strings.ReplaceAll(strings.TrimLeft(strings.Join([]string{prefix, project, stack, ""}, "/"), "/"), "/", "_")
}

func ConfigName(prefix, project, stack, key string) string {
	return ConfigPrefix(prefix, project, stack) + key
}
