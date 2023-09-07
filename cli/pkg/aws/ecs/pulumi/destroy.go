package pulumi

import (
	"context"
	"os"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
)

func (a *PulumiEcs) TearDown(ctx context.Context) error {
	s, err := auto.SelectStackInlineSource(ctx, a.stack, projectName, a.deployFunc)
	if err != nil {
		return err
	}

	if _, err := s.Destroy(ctx, optdestroyColor(a.color), optdestroy.ProgressStreams(os.Stdout)); err != nil {
		return err
	}

	return s.Workspace().RemoveStack(ctx, a.stack)
}

type optdestroyColor Color

func (oc optdestroyColor) ApplyOption(opts *optdestroy.Options) {
	opts.Color = string(oc)
}
