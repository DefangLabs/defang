package do

import (
	"context"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/digitalocean/godo"
)

type byocServerStream struct {
	ctx      context.Context
	err      error
	errCh    <-chan error
	etag     string
	response *defangv1.TailResponse
	services []string
	stream   *godo.AppLogs
}

func newByocServerStream(ctx context.Context, stream godo.AppLogs, services []string) *byocServerStream {

	return &byocServerStream{
		ctx: ctx,
	}
}

func (bs *byocServerStream) Msg() *defangv1.TailResponse {
	return bs.response
}

func (bs *byocServerStream) Receive() bool {
	return true
}

func (bs *byocServerStream) Err() error {
	return bs.err
}

func (bs *byocServerStream) Close() error {
	return nil
}
