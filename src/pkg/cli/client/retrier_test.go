package client

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
)

type grpcMockHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
	getTries    int
	deployTries int
	tailTries   int
}

func (g *grpcMockHandler) Get(context.Context, *connect.Request[defangv1.ServiceID]) (*connect.Response[defangv1.ServiceInfo], error) {
	println(time.Now().Format(time.RFC3339Nano), "Get")
	g.getTries++
	return nil, connect.NewError(connect.CodeUnavailable, errors.New("unavailable"))
}

func (g *grpcMockHandler) Deploy(context.Context, *connect.Request[defangv1.DeployRequest]) (*connect.Response[defangv1.DeployResponse], error) {
	println(time.Now().Format(time.RFC3339Nano), "Deploy")
	g.deployTries++
	return nil, connect.NewError(connect.CodeUnavailable, errors.New("unavailable"))
}

func (g *grpcMockHandler) Tail(ctx context.Context, r *connect.Request[defangv1.TailRequest], s *connect.ServerStream[defangv1.TailResponse]) error {
	println(time.Now().Format(time.RFC3339Nano), "Tail")
	g.tailTries++
	return connect.NewError(connect.CodeUnavailable, errors.New("unavailable"))
}

func TestRetrier(t *testing.T) {
	fabricServer := &grpcMockHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)

	server := httptest.NewTLSServer(handler)
	defer server.Close()

	fabricClient := defangv1connect.NewFabricControllerClient(server.Client(), server.URL, connect.WithGRPC(), connect.WithInterceptors(Retrier{}))

	t.Run("Unary idempotent", func(t *testing.T) {
		_, err := fabricClient.Get(context.Background(), connect.NewRequest(&defangv1.ServiceID{}))
		if err == nil {
			t.Fatal("expected error")
		}
		if fabricServer.getTries != 2 {
			t.Fatalf("expected 2 tries, got %d", fabricServer.getTries)
		}
	})

	t.Run("Unary", func(t *testing.T) {
		_, err := fabricClient.Deploy(context.Background(), connect.NewRequest(&defangv1.DeployRequest{}))
		if err == nil {
			t.Fatal("expected error")
		}
		if fabricServer.deployTries != 2 {
			t.Fatalf("expected 2 tries, got %d", fabricServer.deployTries)
		}
	})

	t.Run("Streaming", func(t *testing.T) {
		ss, err := fabricClient.Tail(context.Background(), connect.NewRequest(&defangv1.TailRequest{}))
		if err != nil {
			t.Fatal(err)
		}
		defer ss.Close()
		if ss.Receive() == true {
			t.Error("expected false")
		}
		if ss.Err() == nil {
			t.Errorf("expected error")
		}
		// TODO: implement retries for streaming calls
		if fabricServer.tailTries != 1 {
			t.Fatalf("expected 1 tries, got %d", fabricServer.tailTries)
		}
	})
}
