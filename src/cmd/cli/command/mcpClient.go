package command

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/term"
)

// ClientProcessMap maps client names to their process names
var ClientProcessMap = map[string]string{
	"claude":   "Claude",
	"windsurf": "Codeium",
	"cursor":   "Cursor",
	"vscode":   "Code",
}

// IsClientRunning checks if a client process is currently running
func IsClientRunning(client string) bool {
	processName := getProcessName(client)

	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq "+processName+".exe", "/NH")
		output, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(output), processName+".exe")
	case "darwin":
		cmd := exec.Command("pgrep", "-x", processName)
		output, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.TrimSpace(string(output)) != ""
	case "linux":
		cmd := exec.Command("pgrep", "-f", strings.ToLower(processName))
		output, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.TrimSpace(string(output)) != ""
	default:
		return false
	}
}

// RestartClient attempts to restart the specified client application
func RestartClient(client string) error {
	processName := getProcessName(client)

	// Kill the process
	var killCmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		killCmd = exec.Command("taskkill", "/F", "/IM", processName+".exe")
	case "darwin":
		killCmd = exec.Command("killall", processName)
	case "linux":
		killCmd = exec.Command("pkill", "-f", strings.ToLower(processName))
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	if err := killCmd.Run(); err != nil {
		return fmt.Errorf("failed to kill %s: %w", processName, err)
	}

	// Wait a bit before restarting
	time.Sleep(2 * time.Second)

	// Start the process again
	var startCmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		startCmd = exec.Command("cmd", "/c", "start", "", processName+".exe")
	case "darwin":
		startCmd = exec.Command("open", "-a", processName)
	case "linux":
		startCmd = exec.Command(strings.ToLower(processName))
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	if err := startCmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w", processName, err)
	}

	fmt.Printf("%s has been restarted.\n", processName)
	return nil
}

// PromptForRestart checks if the client is running and prompts the user to restart it
func PromptForRestart(client string) error {
	if !IsClientRunning(client) {
		return nil
	}

	var shouldRestart bool
	prompt := &survey.Confirm{
		Message: fmt.Sprintf("Would you like to restart %s now?", client),
		Default: true,
	}

	if err := survey.AskOne(prompt, &shouldRestart, survey.WithStdio(term.DefaultTerm.Stdio())); err != nil {
		return fmt.Errorf("failed to prompt for restart: %w", err)
	}

	if shouldRestart {
		fmt.Printf("Restarting %s app...\n", client)
		return RestartClient(client)
	}

	return nil
}

// getProcessName returns the process name for a given client
func getProcessName(client string) string {
	if processName, ok := ClientProcessMap[client]; ok {
		return processName
	}
	return client
}
