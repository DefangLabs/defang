package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/term"
	"github.com/defang-io/defang/src/pkg/types"
)

func BootstrapCommand(ctx context.Context, client client.Client, command string) error {
	projectName, err := client.LoadProjectName()
	if err != nil {
		return err
	}

	term.Debug(" - Running CD command", command, "in project", projectName)
	if DoDryRun {
		return ErrDryRun
	}
	since := time.Now()
	etag, err := client.BootstrapCommand(ctx, command)
	if err != nil || etag == "" {
		return err
	}
	params := TailOptions{
		Service: "",
		Etags:   []types.ETag{etag},
		Since:   since,
		Raw:     false,
	}

	return Tail(ctx, client, params)
}

func BootstrapList(ctx context.Context, client client.Client) error {
	term.Debug(" - Running CD list")
	if DoDryRun {
		return ErrDryRun
	}
	stacks, err := client.BootstrapList(ctx)
	if err != nil {
		return err
	}
	for _, stack := range stacks {
		fmt.Println(" -", stack)
	}
	return nil
}
