package auth

import (
	"context"
	"strings"

	"github.com/bufbuild/connect-go"
)

const XDefangTenantID = "X-Defang-Tenant-Id"

type authInterceptor struct {
	authorization string
	tenantID      string
}

func NewAuthInterceptor(token, tenantID string) connect.Interceptor {
	// Only tenant ID/name travels over the wire now; org header is retired.
	return &authInterceptor{"Bearer " + strings.TrimSpace(token), tenantID}
}

func (a *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set("Authorization", a.authorization)
		req.Header().Set("Content-Type", "application/grpc") // same as the gRPC client
		if a.tenantID != "" {
			req.Header().Set(XDefangTenantID, a.tenantID)
		}
		return next(ctx, req)
	}
}

func (a *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("Authorization", a.authorization)
		conn.RequestHeader().Set("Content-Type", "application/grpc") // same as the gRPC client
		if a.tenantID != "" {
			conn.RequestHeader().Set(XDefangTenantID, a.tenantID)
		}
		return conn
	}
}

func (authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
