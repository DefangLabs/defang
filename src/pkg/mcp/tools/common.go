package tools

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
)

func configureLoader(request mcp.CallToolRequest) *compose.Loader {
	projectName, err := request.RequireString("project_name")
	if err == nil {
		term.Debugf("Project name provided: %s", projectName)
		term.Debug("Function invoked: compose.NewLoader")
		return compose.NewLoader(compose.WithProjectName(projectName))
	}
	arguments := request.GetArguments()
	composeFilePathsArgs, ok := arguments["compose_file_paths"]
	if ok {
		composeFilePaths, ok := composeFilePathsArgs.([]string)
		if ok {
			term.Debugf("Compose file paths provided: %s", composeFilePaths)
			term.Debug("Function invoked: compose.NewLoader")
			return compose.NewLoader(compose.WithPath(composeFilePaths...))
		}
	}

	//TODO: Talk about using both project name and compose file paths
	// if projectNameOK && composeFilePathOK {
	// 	term.Infof("Compose file paths and project name provided: %s, %s", composeFilePaths, projectName)
	// 	return compose.NewLoader(compose.WithProjectName(projectName), compose.WithPath(composeFilePaths...))
	// }

	term.Debug("Function invoked: compose.NewLoader")
	return compose.NewLoader()
}

// HandleTermsOfServiceError checks if the error is related to terms of service not being accepted
// and returns an appropriate error message if it is.
// Returns nil if the error is not related to terms of service.
func HandleTermsOfServiceError(err error) *mcp.CallToolResult {
	if connect.CodeOf(err) == connect.CodeFailedPrecondition && strings.Contains(err.Error(), "terms of service") {
		mcpResult := mcp.NewToolResultErrorFromErr("The operation failed because the terms of service were not accepted. Please accept the terms of service by logging in here: https://portal.defang.io/auth/login. Then try again.", err)
		term.Debugf("MCP output error: %v", mcpResult)
		return mcpResult
	}
	return nil
}

func HandleConfigError(err error) *mcp.CallToolResult {
	var missingConfigErr *compose.ErrMissingConfig
	if errors.As(err, &missingConfigErr) {
		mcpResult := mcp.NewToolResultErrorFromErr("The operation failed due to missing configs not being set. Please use the Defang tool called set_config to set the variable.", err)
		term.Debugf("MCP output error: %v", mcpResult)
		return mcpResult
	}
	return nil
}

func CreateRandomConfigValue() string {
	// Note that no error handling is necessary, as Read always succeeds.
	key := make([]byte, 32)
	rand.Read(key)
	str := base64.StdEncoding.EncodeToString(key)
	re := regexp.MustCompile("[+/=]")
	str = re.ReplaceAllString(str, "")
	return str
}
