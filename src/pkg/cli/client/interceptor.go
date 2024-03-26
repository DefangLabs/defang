package client

import (
	"context"

	"github.com/bufbuild/connect-go"
)

type anonInterceptor struct {
	anonId string
}

func NewAnonInterceptor(anonId string) connect.Interceptor {
	return &anonInterceptor{anonId: anonId}
}

func (a *anonInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set("x-anon-id", a.anonId)
		return next(ctx, req)
	}
}

func (a *anonInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("x-anon-id", a.anonId)
		return conn
	}
}

func (anonInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
