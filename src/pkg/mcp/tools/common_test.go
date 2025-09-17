package tools

import (
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

func TestConfigureLoaderBranches(t *testing.T) {
	makeReq := func(args map[string]any) mcp.CallToolRequest {
		return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
	}
	// project_name path
	loader1 := configureLoader(makeReq(map[string]any{"project_name": "myproj"}))
	assert.NotNil(t, loader1)

	// compose_file_paths path
	loader2 := configureLoader(makeReq(map[string]any{"compose_file_paths": []string{"a.yml", "b.yml"}}))
	assert.NotNil(t, loader2)

	// default path
	loader3 := configureLoader(makeReq(map[string]any{}))
	assert.NotNil(t, loader3)
}

func TestHandleTermsOfServiceError(t *testing.T) {
	origErr := connect.NewError(connect.CodeFailedPrecondition, errors.New("terms of service not accepted"))
	res := HandleTermsOfServiceError(origErr)
	assert.NotNil(t, res)

	otherErr := errors.New("some other error")
	res2 := HandleTermsOfServiceError(otherErr)
	assert.Nil(t, res2)
}

func TestHandleConfigError(t *testing.T) {
	cfgErr := errors.New("missing configs: DB_PASSWORD")
	res := HandleConfigError(cfgErr)
	assert.NotNil(t, res)

	otherErr := errors.New("another error")
	res2 := HandleConfigError(otherErr)
	assert.Nil(t, res2)
}

func TestProviderNotConfiguredError(t *testing.T) {
	// provider auto should error
	err := providerNotConfiguredError(client.ProviderAuto)
	assert.Error(t, err)

	// a real provider (simulate AWS value 'aws') should not error
	var pid client.ProviderID
	_ = pid.Set("aws")
	err2 := providerNotConfiguredError(pid)
	assert.NoError(t, err2)
}
