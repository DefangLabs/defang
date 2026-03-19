package client

import (
	"context"
	"encoding/json"
	"net/http"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const maxPayloadLength = 1024

type grpcLogger struct {
	prefix string
}

func (g grpcLogger) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		// Convert request payload to JSON for logging
		payload, err := json.Marshal(req.Any())
		if err != nil {
			payload = []byte("Error marshaling request payload")
		}

		// Truncate long payloads
		if len(payload) > maxPayloadLength {
			payload = append(payload[:maxPayloadLength], []byte("…")...)
		}

		g.logRequest(req.Header(), req.Spec().Procedure, string(payload))
		return next(ctx, req)
	}
}
func (g grpcLogger) logRequest(header http.Header, reqType, payload string) {
	// Add a request ID to the context
	requestId := pkg.RandomID()
	header.Add("X-Request-Id", requestId)

	term.Debug(g.prefix, requestId, reqType, payload)
}

func (g grpcLogger) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		g.logRequest(conn.RequestHeader(), spec.Procedure, "streaming connection established")
		return conn
	}
}

func (grpcLogger) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
