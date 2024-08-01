package do

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/digitalocean/doctl/pkg/listen"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type byocServerStream struct {
	ctx context.Context
	err error
	// etag     string
	resp     io.ReadCloser
	response *defangv1.TailResponse
	// done     chan struct{}
}

func newByocServerStream(ctx context.Context, liveUrl string) (*byocServerStream, error) {
	url, err := url.Parse(liveUrl)
	if err != nil {
		return nil, err
	}

	schemaFunc := func(message []byte) (io.Reader, error) {
		data := struct {
			Data string `json:"data"`
		}{}
		err = json.Unmarshal(message, &data)
		if err != nil {
			return nil, err
		}
		r := strings.NewReader(data.Data)

		return r, nil
	}

	token := url.Query().Get("token")
	switch url.Scheme {
	case "http":
		url.Scheme = "ws"
	default:
		url.Scheme = "wss"
	}

	listener := listen.NewListener(url, token, schemaFunc, os.Stderr)
	err = listener.Start()
	if err != nil {
		return nil, err
	}

	bs := &byocServerStream{
		ctx: ctx,
		// resp: resp.Body,
	}

	return bs, nil
}

func (bs *byocServerStream) Msg() *defangv1.TailResponse {
	return bs.response
}

func (bs *byocServerStream) Receive() bool {
	<-bs.ctx.Done()
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
