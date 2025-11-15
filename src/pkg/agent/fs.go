package agent

import (
	"encoding/json"
	"os"
	"path/filepath"

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

func CollectFsTools() []ai.Tool {
	return []ai.Tool{
		ai.NewTool[ReadFileParams, string](
			"read_file",
			"Read the contents of a file from the local filesystem",
			func(ctx *ai.ToolContext, params ReadFileParams) (string, error) {
				absPath, err := filepath.Abs(params.Path)
				if err != nil {
					return "", err
				}
				bytes, err := os.ReadFile(absPath)
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
