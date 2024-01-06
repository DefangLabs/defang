package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/aws"
	"github.com/defang-io/defang/src/pkg/aws/ecs/cfn"
)

func Bootstrap(ctx context.Context) error {
	clientCfn := cfn.New("crun-llunesu", aws.Region(pkg.Getenv("AWS_REGION", "us-west-2")))
	if err := clientCfn.SetUp(ctx, "532501343364.dkr.ecr.us-west-2.amazonaws.com/cd:latest", 512_000_000, "linux/amd64"); err != nil {
		return err
	}
	task, err := clientCfn.Run(ctx, nil, "--bootstrap")
	if err != nil {
		return err
	}
	return clientCfn.Tail(ctx, task)
}
