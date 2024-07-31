package do

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/gorilla/websocket"
)

type byocServerStream struct {
	ctx      context.Context
	err      error
	errCh    chan error
	etag     string
	response *defangv1.TailResponse
	conn     *websocket.Conn
	done     chan struct{}
}

func newByocServerStream(ctx context.Context, wsURL string, services []string) (*byocServerStream, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, fmt.Errorf("invalid WebSocket URL: %w", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	bs := &byocServerStream{
		ctx:   ctx,
		errCh: make(chan error, 1),
		conn:  conn,
		done:  make(chan struct{}),
	}

	go bs.readPump()

	return bs, nil
}

func (bs *byocServerStream) readPump() {
	defer close(bs.done)
	for {
		_, message, err := bs.conn.ReadMessage()
		if err != nil {
			bs.errCh <- err
			return
		}

		var response defangv1.TailResponse
		if err := json.Unmarshal(message, &response); err != nil {
			bs.errCh <- err
			return
		}

		bs.response = &response
	}
}

func (bs *byocServerStream) Msg() *defangv1.TailResponse {
	return bs.response
}

func (bs *byocServerStream) Receive() bool {
	select {
	case <-bs.ctx.Done():
		return false
	case err := <-bs.errCh:
		bs.err = err
		return false
	case <-bs.done:
		return false
	default:
		return bs.response != nil
	}
}

func (bs *byocServerStream) Err() error {
	return bs.err
}

func (bs *byocServerStream) Close() error {
	if bs.conn != nil {
		return bs.conn.Close()
	}
	return nil
}
