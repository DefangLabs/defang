package auth

import (
	"context"
	"strings"

	"github.com/bufbuild/connect-go"
)

const (
	XDefangOrgID    = "X-Defang-Orgid"
	XDefangTenantID = "X-Defang-Tenant-Id"
)

type authInterceptor struct {
	authorization string
	orgID         string
}

func NewAuthInterceptor(token, orgID string) connect.Interceptor {
	return &authInterceptor{"Bearer " + strings.TrimSpace(token), orgID}
}

func (a *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set("Authorization", a.authorization)
		req.Header().Set("Content-Type", "application/grpc") // same as the gRPC client
		req.Header().Set(XDefangOrgID, a.orgID)
		if tid := GetSelectedTenantID(); tid != "" {
			req.Header().Set(XDefangTenantID, tid)
		}
		return next(ctx, req)
	}
}

func (a *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("Authorization", a.authorization)
		conn.RequestHeader().Set("Content-Type", "application/grpc") // same as the gRPC client
		conn.RequestHeader().Set(XDefangOrgID, a.orgID)
		if tid := GetSelectedTenantID(); tid != "" {
			conn.RequestHeader().Set(XDefangTenantID, tid)
		}
		return conn
	}
}

func (authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
