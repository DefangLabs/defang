package client

import (
	"context"
	"encoding/json"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
)

const maxPayloadLength = 1024

type grpcLogger struct {
	prefix string
}

func (g grpcLogger) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		// Add a request ID to the context
		requestId := pkg.RandomID()
		req.Header().Add("X-Request-Id", requestId)

		// Get the request type name
		reqType := req.Spec().Procedure

		// Convert request payload to JSON for logging
		payload, err := json.Marshal(req.Any())
		if err != nil {
			payload = []byte("Error marshaling request payload")
		}

		// Truncate long payloads
		if len(payload) > maxPayloadLength {
			payload = append(payload[:maxPayloadLength], []byte("…")...)
		}

		term.Debug(g.prefix, requestId, reqType, string(payload))
		return next(ctx, req)
	}
}

func (g grpcLogger) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)

		// Add a request ID to the context
		requestId := pkg.RandomID()
		conn.RequestHeader().Add("X-Request-Id", requestId)

		// Get the request type name
		reqType := spec.Procedure

		term.Debug(g.prefix, requestId, reqType, "streaming connection established")
		return conn
	}
}

func (grpcLogger) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
