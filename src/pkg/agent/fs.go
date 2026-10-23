package agent

import (
	"encoding/json"
	"errors"
	"io/fs"
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

// openCwdRoot returns a root-constrained name for path. The returned Root, rather
// than this path conversion, enforces that filesystem operations cannot escape cwd.
func openCwdRoot(path string) (*os.Root, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}

	root, err := os.OpenRoot(cwd)
	if err != nil {
		return nil, "", err
	}

	if filepath.IsAbs(path) {
		path, err = filepath.Rel(cwd, path)
		if err != nil {
			_ = root.Close()
			return nil, "", err
		}
	}

	return root, path, nil
}

func rootPathError(op, path string, err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return &os.PathError{Op: op, Path: path, Err: pathErr.Err}
	}
	return err
}

func CollectFsTools() []ai.Tool {
	return []ai.Tool{
		ai.NewTool(
			"read_file",
			"Read the contents of a file from the local filesystem",
			func(ctx *ai.ToolContext, params ReadFileParams) (string, error) {
				root, path, err := openCwdRoot(params.Path)
				if err != nil {
					return "", err
				}
				defer root.Close()

				bytes, err := root.ReadFile(path)
				if err != nil {
					return "", rootPathError("open", params.Path, err)
				}
				return string(bytes), nil
			},
		),
		ai.NewTool(
			"find_files",
			"Find files in a directory on the local filesystem matching a given pattern",
			func(ctx *ai.ToolContext, params FindFilesParams) (string, error) {
				root, path, err := openCwdRoot(params.Path)
				if err != nil {
					return "", err
				}
				defer root.Close()

				var matches []string
				err = fs.WalkDir(root.FS(), filepath.ToSlash(path), func(path string, entry fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					matched, err := filepath.Match(params.Pattern, entry.Name())
					if err != nil {
						return err
					}
					if matched {
						matchPath := filepath.FromSlash(path)
						if filepath.IsAbs(params.Path) {
							matchPath = filepath.Join(root.Name(), matchPath)
						}
						matches = append(matches, matchPath)
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
		ai.NewTool(
			"list_files",
			"List files in a directory on the local filesystem",
			func(ctx *ai.ToolContext, params ListFilesParams) (string, error) {
				root, path, err := openCwdRoot(params.Path)
				if err != nil {
					return "", err
				}
				defer root.Close()

				entries, err := fs.ReadDir(root.FS(), filepath.ToSlash(path))
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
