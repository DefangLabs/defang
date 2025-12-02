package common

import (
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureLoaderBranches(t *testing.T) {
	makeReq := func(args map[string]any) mcp.CallToolRequest {
		return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
	}

	// project_name path
	loader1, err := ConfigureLoader(makeReq(map[string]any{"working_directory": "/tmp", "project_name": "myproj"}))
	require.NoError(t, err)
	assert.NotNil(t, loader1)

	// compose_file_paths path
	loader2, err := ConfigureLoader(makeReq(map[string]any{"working_directory": "/tmp", "compose_file_paths": []string{"a.yml", "b.yml"}}))
	require.NoError(t, err)
	assert.NotNil(t, loader2)

	// default path (no working_directory)
	loader3, err := ConfigureLoader(makeReq(map[string]any{}))
	require.Error(t, err)
	assert.Nil(t, loader3)
}

func TestFixupConfigError(t *testing.T) {
	cfgErr := errors.New("missing configs: DB_PASSWORD")
	newErr := FixupConfigError(cfgErr)
	assert.EqualError(t, newErr, "The operation failed due to missing configs not being set, use the Defang tool called set_config to set the variable: missing configs: DB_PASSWORD")

	otherErr := errors.New("another error")
	res2 := FixupConfigError(otherErr)
	assert.EqualError(t, res2, otherErr.Error())
}

func TestProviderNotConfiguredError(t *testing.T) {
	// provider auto should error
	err := ProviderNotConfiguredError(client.ProviderAuto)
	assert.Error(t, err)

	// a real provider (simulate AWS value 'aws') should not error
	var pid client.ProviderID
	_ = pid.Set("aws")
	err2 := ProviderNotConfiguredError(pid)
	require.NoError(t, err2)
}
