package client

import (
	"context"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/bufbuild/connect-go"
)

type Retrier struct{}

func (Retrier) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		res, err := next(ctx, req)
		if connect.CodeOf(err) == connect.CodeUnavailable {
			// Retry once after a 1 second sleep
			if err := pkg.SleepWithContext(ctx, 1*time.Second); err != nil {
				return nil, err
			}
			res, err = next(ctx, req)
		}
		return res, err
	}
}
func (Retrier) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next // TODO: wrap this to handle streaming rpcs like Tail
}

func (Retrier) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
