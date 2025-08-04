package do

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type byocServerStream struct {
	conn *websocket.Conn
	data struct {
		Op   string `json:"op"`   //  aka source "stdout", "stderr"
		Data string `json:"data"` // "component timestamp message\n"
	}
	err  error
	etag types.ETag
}

func newByocServerStream(ctx context.Context, liveUrl string, etag types.ETag) (*byocServerStream, error) {
	if liveUrl == "none" {
		return &byocServerStream{}, nil
	}

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

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, liveURL.String(), nil) // TODO: should we close resp.Body?
	if err != nil {
		return nil, err
	}

	bs := &byocServerStream{
		conn: conn,
		etag: etag,
	}

	return bs, nil
}

func (bs *byocServerStream) Msg() *defangv1.TailResponse {
	parts := strings.SplitN(bs.data.Data, " ", 3)
	ts, _ := time.Parse(time.RFC3339Nano, parts[1]) // TODO: handle error
	return &defangv1.TailResponse{
		Entries: []*defangv1.LogEntry{{
			Message:   parts[2],
			Timestamp: timestamppb.New(ts),
			Stderr:    bs.data.Op != "stdout",
			Service:   parts[0],
			Etag:      bs.etag,
			// Host:      "host1",
		}},
		Service: parts[0],
		Etag:    bs.etag,
		// Host:    "host2",
	}
}

func (bs *byocServerStream) Receive() bool {
	_, message, err := bs.conn.ReadMessage()
	// println("messageType: ", messageType)
	if err != nil {
		bs.err = err
		return false
	}
	// println(string(message))
	if err := json.Unmarshal(message, &bs.data); err != nil {
		bs.err = err
		return false
	}
	return true
}

func (bs *byocServerStream) Err() error {
	var closeErr *websocket.CloseError
	ok := errors.As(bs.err, &closeErr)
	if !ok {
		return bs.err
	}
	if closeErr.Text == "unexpected EOF" {
		return nil
	}
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
