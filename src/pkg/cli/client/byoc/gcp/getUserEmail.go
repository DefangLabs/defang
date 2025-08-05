package gcp

import (
	"fmt"
	"os/exec"
	"strings"
)

func GetUserEmail() (string, error) {
	cmd := exec.Command("gcloud", "config", "get-value", "account")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gcloud error: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
