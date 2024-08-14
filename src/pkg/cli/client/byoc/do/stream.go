package do

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type byocServerStream struct {
	conn *websocket.Conn
	data struct {
		Data string `json:"data"`
	}
	err  error
	etag types.ETag
}

func newByocServerStream(ctx context.Context, liveUrl string, etag types.ETag) (*byocServerStream, error) {
	liveURL, err := url.Parse(liveUrl)
	if err != nil {
		return nil, err
	}

	switch liveURL.Scheme {
	case "http":
		liveURL.Scheme = "ws"
	default:
		liveURL.Scheme = "wss"
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, liveURL.String(), nil)
	if err != nil {
		return nil, err
	}

	bs := &byocServerStream{
		// ctx:    ctx,
		// cancel: cancel,
		conn: conn,
		etag: etag,
	}

	return bs, nil
}

func (bs *byocServerStream) Msg() *defangv1.TailResponse {
	return &defangv1.TailResponse{
		Entries: []*defangv1.LogEntry{{
			Message:   bs.data.Data,
			Timestamp: timestamppb.Now(),
			Stderr:    true,
			Service:   "service1",
			Etag:      bs.etag,
			Host:      "host1",
		}},
		Service: "service2",
		Etag:    bs.etag,
		Host:    "host2",
	}
}

func (bs *byocServerStream) Receive() bool {
	messageType, message, err := bs.conn.ReadMessage()
	println("messageType: ", messageType)
	if err != nil {
		bs.err = err
		return false
	}
	println(string(message))
	if err := json.Unmarshal(message, &bs.data); err != nil {
		bs.err = err
		return false
	}
	return true
}

func (bs *byocServerStream) Err() error {
	return bs.err
}

func (bs *byocServerStream) Close() error {
	writeCloseMessage(bs.conn)
	return bs.conn.Close()
}

func writeCloseMessage(c *websocket.Conn) error {
	err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		return err
	}

	return nil
}
