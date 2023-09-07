package pulumi

import (
	"context"
	"os"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
)

func (a *PulumiEcs) SetUp(ctx context.Context, image string, memory uint64) error {
	a.image = image // TODO: set in stack config
	a.memory = memory

	s, err := a.createStack(ctx)
	if err != nil {
		return err
	}

	res, err := s.Up(ctx, optupColor(a.color), optup.ProgressStreams(os.Stdout))
	if err != nil {
		return err
	}

	a.fillOutputs(res.Outputs)
	return nil
}

type optupColor Color

func (oc optupColor) ApplyOption(opts *optup.Options) {
	opts.Color = string(oc)
}
