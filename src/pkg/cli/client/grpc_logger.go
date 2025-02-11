package client

import (
	"context"
	"encoding/json"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
)

const maxPayloadLength = 1024

type grpcLogger struct {
	prefix string
}

func (g grpcLogger) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
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

		term.Debug(g.prefix, reqType, string(payload))
		return next(ctx, req)
	}
}

func (grpcLogger) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (grpcLogger) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
