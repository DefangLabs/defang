package client

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
)

type grpcMockHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
	tries int
}

func (g *grpcMockHandler) Deploy(context.Context, *connect.Request[defangv1.DeployRequest]) (*connect.Response[defangv1.DeployResponse], error) {
	g.tries++
	return nil, connect.NewError(connect.CodeUnavailable, errors.New("unavailable"))
}

// func (g *grpcMockHandler) Tail(ctx context.Context, r *connect.Request[defangv1.TailRequest], s *connect.ServerStream[defangv1.TailResponse]) error {
// 	g.tries++
// 	return connect.NewError(connect.CodeUnavailable, errors.New("unavailable"))
// }

func TestRetrier(t *testing.T) {
	fabricServer := &grpcMockHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)

	server := httptest.NewTLSServer(handler)
	defer server.Close()

	fabricClient := defangv1connect.NewFabricControllerClient(server.Client(), server.URL, connect.WithGRPC(), connect.WithInterceptors(Retrier{}))

	_, err := fabricClient.Deploy(t.Context(), connect.NewRequest(&defangv1.DeployRequest{}))
	if err == nil {
		t.Fatal("expected error")
	}
	if fabricServer.tries != 2 {
		t.Fatalf("expected 2 tries, got %d", fabricServer.tries)
	}
}
