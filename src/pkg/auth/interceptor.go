package auth

import (
	"context"
	"strings"

	"github.com/bufbuild/connect-go"
)

const TenantHeader = "X-Defang-Tenant-Id"

type authInterceptor struct {
	authorization  string
	tenantNameOrId string
}

func NewAuthInterceptor(token, tenantNameOrId string) connect.Interceptor {
	// Only tenant ID/name travels over the wire now; org header is retired.
	return &authInterceptor{"Bearer " + strings.TrimSpace(token), tenantNameOrId}
}

func (a *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set("Authorization", a.authorization)
		req.Header().Set("Content-Type", "application/grpc") // same as the gRPC client
		if a.tenantNameOrId != "" {
			req.Header().Set(TenantHeader, a.tenantNameOrId)
		}
		return next(ctx, req)
	}
}

func (a *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("Authorization", a.authorization)
		conn.RequestHeader().Set("Content-Type", "application/grpc") // same as the gRPC client
		if a.tenantNameOrId != "" {
			conn.RequestHeader().Set(TenantHeader, a.tenantNameOrId)
		}
		return conn
	}
}

func (authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
