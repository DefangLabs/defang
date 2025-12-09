package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/firebase/genkit/go/ai"
)

type ReadFileParams struct {
	Path string `json:"path"`
}

type FindFilesParams struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern"`
}

type ListFilesParams struct {
	Path string `json:"path"`
}

func isSafePath(path string) bool {
	cleaned := filepath.Clean(path)

	// Reject absolute paths
	if filepath.IsAbs(cleaned) {
		return false
	}

	// Reject paths that traverse outside the current directory
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return false
	}

	return true
}

func CollectFsTools() []ai.Tool {
	return []ai.Tool{
		ai.NewTool[ReadFileParams, string](
			"read_file",
			"Read the contents of a file from the local filesystem",
			func(ctx *ai.ToolContext, params ReadFileParams) (string, error) {
				if !isSafePath(params.Path) {
					return "", errors.New("Accessing files outside the current working directory is not permitted")
				}
				bytes, err := os.ReadFile(params.Path)
				if err != nil {
					return "", err
				}
				return string(bytes), nil
			},
		),
		ai.NewTool[FindFilesParams, string](
			"find_files",
			"Find files in a directory on the local filesystem matching a given pattern",
			func(ctx *ai.ToolContext, params FindFilesParams) (string, error) {
				if !isSafePath(params.Path) {
					return "", errors.New("Accessing files outside the current working directory is not permitted")
				}
				var matches []string
				err := filepath.Walk(params.Path, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					matched, err := filepath.Match(params.Pattern, info.Name())
					if err != nil {
						return err
					}
					if matched {
						matches = append(matches, path)
					}
					return nil
				})
				if err != nil {
					return "", err
				}
				b, err := json.MarshalIndent(matches, "", "  ")
				if err != nil {
					return "", err
				}
				return string(b), nil
			},
		),
		ai.NewTool[ListFilesParams, string](
			"list_files",
			"List files in a directory on the local filesystem",
			func(ctx *ai.ToolContext, params ListFilesParams) (string, error) {
				if !isSafePath(params.Path) {
					return "", errors.New("Accessing files outside the current working directory is not permitted")
				}
				entries, err := os.ReadDir(params.Path)
				if err != nil {
					return "", err
				}
				var files []string
				for _, entry := range entries {
					files = append(files, entry.Name())
				}
				b, err := json.MarshalIndent(files, "", "  ")
				if err != nil {
					return "", err
				}
				return string(b), nil
			},
		),
	}
}
