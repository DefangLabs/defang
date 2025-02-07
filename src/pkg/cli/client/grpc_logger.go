package client

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
)

type grpcLogger struct {
}

func (a grpcLogger) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		// Get the request type name
		reqType := reflect.TypeOf(req.Any()).String()

		// Convert request payload to JSON for logging
		payload, err := json.Marshal(req.Any())
		if err != nil {
			payload = []byte("Error marshaling request payload")
		}

		term.Debug("fabric", reqType, string(payload))

		return next(ctx, req)
	}
}

func (a grpcLogger) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (grpcLogger) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
