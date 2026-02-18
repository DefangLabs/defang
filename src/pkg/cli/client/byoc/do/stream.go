package do

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

// streamLogs connects to a DO live URL websocket and yields TailResponse protos.
func streamLogs(ctx context.Context, liveUrl string, etag types.ETag) (iter.Seq2[*defangv1.TailResponse, error], error) {
	if liveUrl == "none" {
		return func(yield func(*defangv1.TailResponse, error) bool) {}, nil
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

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, liveURL.String(), nil)
	if err != nil {
		return nil, err
	}

	return func(yield func(*defangv1.TailResponse, error) bool) {
		defer func() {
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			conn.Close()
		}()
		var data struct {
			Op   string `json:"op"`
			Data string `json:"data"`
		}
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				var closeErr *websocket.CloseError
				if errors.As(err, &closeErr) && closeErr.Text == "unexpected EOF" {
					return
				}
				yield(nil, err)
				return
			}
			if err := json.Unmarshal(message, &data); err != nil {
				if !yield(nil, err) {
					return
				}
			}
			parts := strings.SplitN(data.Data, " ", 3)
			if len(parts) < 3 {
				// Skip malformed messages
				continue
			}
			ts, _ := time.Parse(time.RFC3339Nano, parts[1])
			resp := &defangv1.TailResponse{
				Entries: []*defangv1.LogEntry{{
					Message:   parts[2],
					Timestamp: timestamppb.New(ts),
					Stderr:    data.Op != "stdout",
					Service:   parts[0],
					Etag:      etag,
				}},
				Service: parts[0],
				Etag:    etag,
			}
			if !yield(resp, nil) {
				return
			}
		}
	}, nil
}
