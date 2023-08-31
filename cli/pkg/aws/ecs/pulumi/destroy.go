package pulumi

import (
	"context"
	"os"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
)

func (a *AwsEcs) Destroy(ctx context.Context, color Color) error {
	s, err := a.createStack(ctx)
	if err != nil {
		return err
	}

	if _, err := s.Destroy(ctx, optdestroyColor(color), optdestroy.ProgressStreams(os.Stdout)); err != nil {
		return err
	}

	return s.Workspace().RemoveStack(ctx, a.stack)
}

type optdestroyColor Color

func (oc optdestroyColor) ApplyOption(opts *optdestroy.Options) {
	opts.Color = string(oc)
}
