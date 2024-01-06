package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/defang-io/defang/src/pkg/cli/client"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func Generate(ctx context.Context, client client.Client, language string, description string) ([]string, error) {
	if DoDryRun {
		Warn(" ! Dry run, not generating files")
		return nil, nil
	}

	response, err := client.GenerateFiles(ctx, &pb.GenerateFilesRequest{
		Language: language,
		Prompt:   description,
	})
	if err != nil {
		return nil, err
	}

	if DoVerbose {
		// Print the files that were generated
		for _, file := range response.Files {
			Debug(file.Name + "\n```")
			Debug(file.Content)
			Debug("```")
			Debug("")
			Debug("")
		}
	}

	// Write each file to disk
	Info(" * Writing files to disk...")
	for _, file := range response.Files {
		// Print the files that were generated
		fmt.Println("   -", file.Name)
		// TODO: this will overwrite existing files
		if err = os.WriteFile(file.Name, []byte(file.Content), 0644); err != nil {
			return nil, err
		}
	}

	// put the file names in an array
	var fileNames []string
	for _, file := range response.Files {
		fileNames = append(fileNames, file.Name)
	}

	return fileNames, nil
}
