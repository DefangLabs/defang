package auth

import (
	"context"
	"strings"

	"github.com/bufbuild/connect-go"
)

type authInterceptor struct {
	authorization string
}

func NewAuthInterceptor(token string) connect.Interceptor {
	return &authInterceptor{"Bearer " + strings.TrimSpace(token)}
}

func (a *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set("authorization", a.authorization)
		req.Header().Set("content-type", "application/grpc") // same as the gRPC client
		return next(ctx, req)
	}
}

func (a *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("authorization", a.authorization)
		conn.RequestHeader().Set("content-type", "application/grpc") // same as the gRPC client
		return conn
	}
}

func (authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
