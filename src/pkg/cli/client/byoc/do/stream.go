package do

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/DefangLabs/defang/src/pkg/http"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type byocServerStream struct {
	// ctx context.Context
	err error
	// etag     string
	resp     io.ReadCloser
	response *defangv1.TailResponse
	// done     chan struct{}
}

func newByocServerStream(ctx context.Context, url string) (*byocServerStream, error) {
	// u, err := url.Parse(wsURL)
	// if err != nil {
	// 	return nil, fmt.Errorf("invalid WebSocket URL: %w", err)
	// }

	// resp, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to connect to WebSocket: %w", err)
	// }

	resp, err := http.GetWithContext(ctx, url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to connect to log stream: %s", resp.Status)
	}

	bs := &byocServerStream{
		// ctx:  ctx,
		resp: resp.Body,
	}

	return bs, nil
}

func (bs *byocServerStream) Msg() *defangv1.TailResponse {
	return bs.response
}

func (bs *byocServerStream) Receive() bool {
	// scanner := bufio.NewScanner(); TODO: probably need to use this
	buffer := &bytes.Buffer{}
	var message [1]byte
	for {
		// _, message, err := bs.resp.ReadMessage(); TODO: websocket support
		n, err := bs.resp.Read(message[:])
		fmt.Println(n)
		if err != nil {
			bs.err = err
			return false
		}
		if message[0] != '\n' {
			buffer.Write(message[:n])
			continue
		}

		bs.response = &defangv1.TailResponse{
			Entries: []*defangv1.LogEntry{
				{
					Message:   buffer.String(),
					Timestamp: timestamppb.Now(), // ??
					Stderr:    true,
				},
			},
		}
		buffer.Reset()
		return true
	}
}

func (bs *byocServerStream) Err() error {
	return bs.err
}

func (bs *byocServerStream) Close() error {
	return bs.resp.Close()
}
