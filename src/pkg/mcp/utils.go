package mcp

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/DefangLabs/defang/src/pkg/mcp/logger"
)

const (
	AskDefangBaseURL      = "https://ask.defang.io"
	DocumentationEndpoint = "data"
	KnowledgeBaseDir      = "./knowledge_base"
)

func SetupKnowledgeBase() error {
	logger.Sugar.Info("Setting up knowledge base")
	filenames := []string{"knowledge_base.json", "samples_examples.json"}

	logger.Sugar.Infof("Attempting to download knowledge base files: %v", filenames)

	// Create knowledge base directory if it doesn't exist
	logger.Sugar.Infof("Creating knowledge base directory: %s", KnowledgeBaseDir)
	if err := os.MkdirAll(KnowledgeBaseDir, 0700); err != nil {
		logger.Sugar.Errorw("Failed to create knowledge base directory", "error", err)
		return err
	}

	for _, filename := range filenames {
		logger.Sugar.Infof("Downloading knowledge base file: %s", filename)
		err := downloadFile(KnowledgeBaseDir+"/"+filename, AskDefangBaseURL+"/"+DocumentationEndpoint+"/"+filename)
		if err != nil {
			logger.Sugar.Errorw("Failed to download knowledge base file", "error", err, "filename", filename)
			return err
		}
	}

	logger.Sugar.Info("Successfully downloaded knowledge base files")
	return nil
}

func downloadFile(filepath string, url string) (err error) {
	// Create the file
	out, err := os.Create(filepath)
	logger.Sugar.Infof("Creating file: %s", filepath)
	if err != nil {
		logger.Sugar.Errorw("Failed to create file", "error", err, "filepath", filepath)
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	logger.Sugar.Infof("Downloading file: %s", url)
	if err != nil {
		logger.Sugar.Errorw("Failed to download file", "error", err, "url", url)
		return err
	}
	defer resp.Body.Close()

	// Check server response
	logger.Sugar.Infof("Checking server response: %s", resp.Status)
	if resp.StatusCode != http.StatusOK {
		logger.Sugar.Errorw("Failed to download file", "error", fmt.Errorf("bad status: %s", resp.Status), "url", url)
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Writer the body to file
	logger.Sugar.Infof("Copying Using IO Copy: %s", filepath)
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		logger.Sugar.Errorw("Failed to write file", "error", err, "filepath", filepath)
		return err
	}

	return nil
}
