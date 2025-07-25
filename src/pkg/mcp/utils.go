package mcp

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const (
	AskDefangBaseURL      = "https://ask.defang.io"
	DocumentationEndpoint = "data"
)

var KnowledgeBaseDir = client.StateDir

func SetupKnowledgeBase() error {
	term.Debug("Setting up knowledge base")
	filenames := []string{"knowledge_base.json", "samples_examples.json"}

	term.Debugf("Attempting to download knowledge base files: %v", filenames)

	// Create knowledge base directory if it doesn't exist
	term.Debugf("Creating knowledge base directory: %s", KnowledgeBaseDir)
	if err := os.MkdirAll(KnowledgeBaseDir, 0700); err != nil {
		term.Error("Failed to create knowledge base directory", "error", err)
		return err
	}

	for _, filename := range filenames {
		term.Debugf("Downloading knowledge base file: %s", filename)
		err := downloadKnowledgeBase(KnowledgeBaseDir+"/"+filename, "/"+DocumentationEndpoint+"/"+filename)
		if err != nil {
			term.Error("Failed to download knowledge base file", "error", err, "filename", filename)
			return err
		}
	}

	term.Debug("Successfully downloaded knowledge base files")
	return nil
}

func downloadKnowledgeBase(filepath string, path string) (err error) {
	// Create the file
	out, err := os.Create(filepath)
	term.Debugf("Creating file: %s", filepath)
	if err != nil {
		term.Error("Failed to create file", "error", err, "filepath", filepath)
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(AskDefangBaseURL + path)
	term.Debugf("Downloading file: %s", path)
	if err != nil {
		term.Error("Failed to download file", "error", err, "url", path)
		return err
	}
	defer resp.Body.Close()

	// Check server response
	term.Debugf("Checking server response: %s", resp.Status)
	if resp.StatusCode != http.StatusOK {
		term.Error("Failed to download file", "error", fmt.Errorf("bad status: %s", resp.Status), "url", path)
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Writer the body to file
	term.Debugf("Copying Using IO Copy: %s", filepath)
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		term.Error("Failed to write file", "error", err, "filepath", filepath)
		return err
	}

	return nil
}
