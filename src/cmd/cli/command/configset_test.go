package command

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigSetCommand_MultipleValues(t *testing.T) {
	// Test that the command accepts multiple KEY=VALUE arguments
	cmd := configSetCmd

	// Test valid multiple arguments
	err := cmd.ValidateArgs([]string{"KEY1=value1", "KEY2=value2", "KEY3=value3"})
	if err != nil {
		t.Errorf("Expected no error for multiple KEY=VALUE arguments, got: %v", err)
	}

	// Test single argument
	err = cmd.ValidateArgs([]string{"KEY1=value1"})
	if err != nil {
		t.Errorf("Expected no error for single KEY=VALUE argument, got: %v", err)
	}

	// Test no arguments (should fail)
	err = cmd.ValidateArgs([]string{})
	if err == nil {
		t.Error("Expected error for no arguments, got nil")
	}
}

func TestConfigSetCommand_FromEnvFlag(t *testing.T) {
	// Create a temporary .env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	envContent := `KEY1=value1
KEY2=value2
KEY3=value3
`
	err := os.WriteFile(envFile, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test .env file: %v", err)
	}

	// Verify the flag is registered
	flag := configSetCmd.Flags().Lookup("from-env")
	if flag == nil {
		t.Error("Expected --from-env flag to be registered")
	}
}

func TestConfigSetCommand_BackwardCompatibility(t *testing.T) {
	// Test that the old behavior still works
	cmd := configSetCmd

	// Test single config name (backward compat)
	err := cmd.ValidateArgs([]string{"MYCONFIG"})
	if err != nil {
		t.Errorf("Expected no error for single config name, got: %v", err)
	}

	// Test config name with file (backward compat)
	err = cmd.ValidateArgs([]string{"MYCONFIG", "file.txt"})
	if err != nil {
		t.Errorf("Expected no error for config name with file, got: %v", err)
	}
}
