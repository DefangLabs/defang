package detector

import (
	"os"
	"strings"
)

type RuntimeEnv string

const (
	EnvLocal          RuntimeEnv = "local"
	EnvCodespaces     RuntimeEnv = "codespaces"
	EnvDevContainer   RuntimeEnv = "devcontainer"
	EnvOtherContainer RuntimeEnv = "container"
)

// DetectRuntimeEnv tries to recognize Codespaces / Dev Container / generic container.
func DetectRuntimeEnv() RuntimeEnv {
	// Explicit Codespaces env vars
	if os.Getenv("CODESPACES") == "true" || os.Getenv("CODESPACE_NAME") != "" {
		return EnvCodespaces
	}
	// Common Dev Container envs
	if os.Getenv("VSCODE_REMOTE_CONTAINERS") == "true" ||
		os.Getenv("REMOTE_CONTAINERS") == "true" ||
		os.Getenv("DEVCONTAINER") == "true" {
		return EnvDevContainer
	}
	// Generic container heuristics
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		s := string(data)
		if strings.Contains(s, "docker") || strings.Contains(s, "containerd") || strings.Contains(s, "kubepods") {
			return EnvOtherContainer
		}
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return EnvOtherContainer
	}
	return EnvLocal
}
