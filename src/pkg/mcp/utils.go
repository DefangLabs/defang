package mcp

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

const DocumentationEndpoint = "data"

// AskDefangBaseURL is a var so tests can override it
var AskDefangBaseURL = "https://ask.defang.io"
var KnowledgeBaseDir = client.StateDir

// knowledgeBaseFilenames is the ordered list of knowledge base files to download as a fixed-size array.
var knowledgeBaseFilenames = [...]string{"knowledge_base.json", "samples_examples.json"}

func SetupKnowledgeBase() error {
	slog.Debug("Setting up knowledge base")
	slog.Debug(fmt.Sprintf("Attempting to download knowledge base files: %v", knowledgeBaseFilenames))

	// Create knowledge base directory if it doesn't exist
	slog.Debug("Creating knowledge base directory: " + KnowledgeBaseDir)
	if err := os.MkdirAll(KnowledgeBaseDir, 0700); err != nil {
		slog.Error(fmt.Sprint("Failed to create knowledge base directory", "error", err))
		return err
	}

	for _, filename := range knowledgeBaseFilenames {
		slog.Debug("Downloading knowledge base file: " + filename)
		err := downloadKnowledgeBase(KnowledgeBaseDir+"/"+filename, "/"+DocumentationEndpoint+"/"+filename)
		if err != nil {
			slog.Error(fmt.Sprint("Failed to download knowledge base file", "error", err, "filename", filename))
			return err
		}
	}

	slog.Debug("Successfully downloaded knowledge base files")
	return nil
}

func downloadKnowledgeBase(filepath string, path string) (err error) {
	// Create the file
	out, err := os.Create(filepath)
	slog.Debug("Creating file: " + filepath)
	if err != nil {
		slog.Error(fmt.Sprint("Failed to create file", "error", err, "filepath", filepath))
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(AskDefangBaseURL + path)
	slog.Debug("Downloading file: " + path)
	if err != nil {
		slog.Error(fmt.Sprint("Failed to download file", "error", err, "url", path))
		return err
	}
	defer resp.Body.Close()

	// Check server response
	slog.Debug("Checking server response: " + resp.Status)
	if resp.StatusCode != http.StatusOK {
		slog.Error(fmt.Sprint("Failed to download file", "error", fmt.Errorf("bad status: %s", resp.Status), "url", path))
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Writer the body to file
	slog.Debug("Copying Using IO Copy: " + filepath)
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		slog.Error(fmt.Sprint("Failed to write file", "error", err, "filepath", filepath))
		return err
	}

	return nil
}
