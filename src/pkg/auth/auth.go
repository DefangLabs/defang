package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/bufbuild/connect-go"
	"google.golang.org/grpc/credentials"
)

func ParseAuthorizationHeader(header string) (id string, token string) {
	tokens := strings.SplitN(header, " ", 2)
	id = tokens[0]
	if len(tokens) == 2 {
		token = tokens[1]
	}
	return
}

func ParseBearerToken(header string) string {
	id, token := ParseAuthorizationHeader(header)
	if id != "Bearer" && id != "bearer" {
		return ""
	}
	return token
}

type bearerTokenCreds map[string]string

func NewBearerTokenCreds(token string) credentials.PerRPCCredentials {
	return bearerTokenCreds{
		"authorization": "Bearer " + strings.TrimSpace(token),
	}
}

func (bt bearerTokenCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return bt, nil
}

func (bt bearerTokenCreds) RequireTransportSecurity() bool {
	return true
}

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

func GetBearerTokenFromHeader(header http.Header) (string, error) {
	value := header.Get("authorization")
	if value == "" {
		return "", errors.New("missing authorization")
	}

	// Parse authorization header "Bearer <access_token>"
	access_token := ParseBearerToken(value)
	if access_token == "" {
		return "", errors.New("missing bearer token")
	}
	return access_token, nil
}
