package clouds

import (
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/clouds/aws/ecs"
	"github.com/defang-io/defang/src/pkg/logs"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// byocServerStream is a wrapper around awsecs.EventStream that implements connect-like ServerStream
type byocServerStream struct {
	cancelTaskCh func()
	err          error
	errCh        <-chan error
	etag         string
	response     *defangv1.TailResponse
	service      string
	stream       ecs.EventStream
	taskCh       <-chan error
}

var _ client.ServerStream[defangv1.TailResponse] = (*byocServerStream)(nil)

func (bs *byocServerStream) Close() error {
	if bs.cancelTaskCh != nil {
		bs.cancelTaskCh()
	}
	return bs.stream.Close()
}

func (bs *byocServerStream) Err() error {
	if bs.err == io.EOF {
		return nil // same as the original gRPC/connect server stream
	}
	return annotateAwsError(bs.err)
}

func (bs *byocServerStream) Msg() *defangv1.TailResponse {
	return bs.response
}

type hasErrCh interface {
	Errs() <-chan error
}

func (bs *byocServerStream) Receive() bool {
	select {
	case e := <-bs.stream.Events(): // blocking
		events, err := ecs.GetLogEvents(e)
		if err != nil {
			bs.err = err
			return false
		}
		bs.response = &defangv1.TailResponse{}
		if len(events) == 0 {
			// The original gRPC/connect server stream would never send an empty response.
			// We could loop around the select, but returning an empty response updates the spinner.
			return true
		}
		var record logs.FirelensMessage
		parseFirelensRecords := false
		// Get the Etag/Host/Service from the first event (should be the same for all events in this batch)
		event := events[0]
		if parts := strings.Split(*event.LogStreamName, "/"); len(parts) == 3 {
			if strings.Contains(*event.LogGroupIdentifier, ":"+CdTaskPrefix) {
				// These events are from the CD task: "crun/main/taskID" stream; we should detect stdout/stderr
				bs.response.Etag = bs.etag // pass the etag filter below, but we already filtered the tail by taskID
				bs.response.Host = "pulumi"
				bs.response.Service = "cd"
			} else {
				// These events are from an awslogs service task: "tenant/service_etag/taskID" stream
				bs.response.Host = parts[2] // TODO: figure out actual hostname/IP
				parts = strings.Split(parts[1], "_")
				if len(parts) != 2 || !pkg.IsValidRandomID(parts[1]) {
					// skip, ignore sidecar logs (like route53-sidecar or fluentbit)
					return true
				}
				service, etag := parts[0], parts[1]
				bs.response.Etag = etag
				bs.response.Service = service
			}
		} else if strings.Contains(*event.LogStreamName, "-firelens-") {
			// These events are from the Firelens sidecar; try to parse the JSON
			if err := json.Unmarshal([]byte(*event.Message), &record); err == nil {
				bs.response.Etag = record.Etag
				bs.response.Host = record.Host             // TODO: use "kaniko" for kaniko logs
				bs.response.Service = record.ContainerName // TODO: could be service_etag
				parseFirelensRecords = true
			}
		}
		if bs.etag != "" && bs.etag != bs.response.Etag {
			return true // TODO: filter these out using the AWS StartLiveTail API
		}
		if bs.service != "" && bs.service != bs.response.Service {
			return true // TODO: filter these out using the AWS StartLiveTail API
		}
		entries := make([]*defangv1.LogEntry, len(events))
		for i, event := range events {
			stderr := false //  TODO: detect somehow from source
			message := *event.Message
			if parseFirelensRecords {
				if err := json.Unmarshal([]byte(message), &record); err == nil {
					message = record.Log
					if record.ContainerName == "kaniko" {
						stderr = logs.IsLogrusError(message)
					} else {
						stderr = record.Source == logs.SourceStderr
					}
				}
			} else if bs.response.Service == "cd" && strings.HasPrefix(message, " ** ") {
				stderr = true
			}
			entries[i] = &defangv1.LogEntry{
				Message:   message,
				Stderr:    stderr,
				Timestamp: timestamppb.New(time.UnixMilli(*event.Timestamp)),
			}
		}
		bs.response.Entries = entries
		return true

	case err := <-bs.errCh: // blocking (if not nil)
		bs.err = err
		return false // abort on first error?

	case err := <-bs.taskCh: // blocking (if not nil)
		bs.err = err
		return false // TODO: make sure we got all the logs from the task
	}
}
