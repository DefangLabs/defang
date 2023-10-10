package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/bufbuild/connect-go"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
)

func Generate(ctx context.Context, client defangv1connect.FabricControllerClient, language string, description string) ([]string, error) {
	if DoDryRun {
		Warn(" ! Dry run, not generating files")
		return nil, nil
	}

	response, err := client.GenerateFiles(ctx, connect.NewRequest(&pb.GenerateFilesRequest{
		Language: language,
		Prompt:   description,
	}))
	if err != nil {
		return nil, err
	}

	if DoVerbose {
		// Print the files that were generated
		for _, file := range response.Msg.Files {
			Debug(file.Name + "\n```")
			Debug(file.Content)
			Debug("```")
			Debug("")
			Debug("")
		}
	}

	// Write each file to disk
	Info(" * Writing files to disk...")
	for _, file := range response.Msg.Files {
		// Print the files that were generated
		fmt.Println("   -", file.Name)
		// TODO: this will overwrite existing files
		if err = os.WriteFile(file.Name, []byte(file.Content), 0644); err != nil {
			return nil, err
		}
	}

	// put the file names in an array
	var fileNames []string
	for _, file := range response.Msg.Files {
		fileNames = append(fileNames, file.Name)
	}

	return fileNames, nil
}
