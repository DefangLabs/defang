package auth

import (
	"context"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/types"
)

const TenantHeader = "X-Defang-Tenant-Id"

type authInterceptor struct {
	authorization   string
	requestedTenant types.TenantNameOrID
	userAgent       string
}

func NewAuthInterceptor(token string, requestedTenant types.TenantNameOrID, userAgent string) connect.Interceptor {
	// Only tenant ID/name travels over the wire now; org header is retired.
	return &authInterceptor{"Bearer " + strings.TrimSpace(token), requestedTenant, userAgent}
}

func (a *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		a.setHeaders(req.Header())
		return next(ctx, req)
	}
}

func (a *authInterceptor) setHeaders(header http.Header) {
	if a.userAgent != "" {
		header.Set("User-Agent", a.userAgent)
	}
	header.Set("Authorization", a.authorization)
	header.Set("Content-Type", "application/grpc") // same as the gRPC client
	if a.requestedTenant.IsSet() {
		header.Set(TenantHeader, string(a.requestedTenant))
	}
}

func (a *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		a.setHeaders(conn.RequestHeader())
		return conn
	}
}

func (authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
