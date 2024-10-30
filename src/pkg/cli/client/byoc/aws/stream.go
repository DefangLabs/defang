package aws

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type SSMode uint8

const (
	SSModeUnknown SSMode = iota
	SSModeAwslogs
	SSModeCd
	SSModeFirelens
	SSModeECS
)

type TailResponseHeader struct {
	Service string
	Host    string
	Etag    string
	Mode    SSMode
}

// byocServerStream is a wrapper around awsecs.EventStream that implements connect-like ServerStream
type byocServerStream struct {
	ctx      context.Context
	err      error
	etag     string
	response *defangv1.TailResponse
	services []string
	stream   ecs.EventStream

	ecsEventsHandler ECSEventHandler
}

func newByocServerStream(ctx context.Context, stream ecs.EventStream, etag string, services []string, ecsEventHandler ECSEventHandler) *byocServerStream {
	return &byocServerStream{
		ctx:      ctx,
		etag:     etag,
		stream:   stream,
		services: services,

		ecsEventsHandler: ecsEventHandler,
	}
}

var _ client.ServerStream[defangv1.TailResponse] = (*byocServerStream)(nil)

func (bs *byocServerStream) Close() error {
	return bs.stream.Close()
}

func (bs *byocServerStream) Err() error {
	if bs.err == io.EOF {
		return nil // same as the original gRPC/connect server stream
	}
	return byoc.AnnotateAwsError(bs.err)
}

func (bs *byocServerStream) Msg() *defangv1.TailResponse {
	return bs.response
}

func (bs *byocServerStream) Receive() bool {
	var events []ecs.LogEvent
	select {
	case e := <-bs.stream.Events(): // blocking
		if bs.stream.Err() != nil {
			bs.err = bs.stream.Err()
			return false
		}
		var err error
		events, err = ecs.GetLogEvents(e)
		if err != nil {
			bs.err = err
			return false
		}
	case <-bs.ctx.Done(): // blocking (if not nil)
		bs.err = context.Cause(bs.ctx)
		return false
	}

	if len(events) == 0 {
		// The original gRPC/connect server stream would never send an empty response.
		// We could loop around the select, but returning an empty response updates the spinner.
		return true
	}

	// Get the Etag/Host/Service from the first event (should be the same for all events in this batch)
	header := bs.makeResponseHeader(events[0])
	// if unable to make a header, skip this stream
	if header == nil {
		return true
	}

	bs.response = bs.makeResponse(header, events)

	return true
}

func (bs *byocServerStream) makeResponseHeader(event ecs.LogEvent) *TailResponseHeader {
	var header TailResponseHeader

	parts := strings.Split(*event.LogStreamName, "/")

	if len(parts) == 3 {
		if strings.Contains(*event.LogGroupIdentifier, ":"+byoc.CdTaskPrefix) {
			// These events are from the CD task: "crun/main/taskID" stream; we should detect stdout/stderr
			header.Etag = bs.etag // pass the etag filter below, but we already filtered the tail by taskID
			header.Host = "pulumi"
			header.Service = "cd"
			header.Mode = SSModeCd

			return &header
		}

		// These events are from an awslogs service task with a stream name like "service/service_etag/taskID"
		header.Host = parts[2] // TODO: figure out actual hostname/IP
		seParts := strings.Split(parts[1], "_")
		if len(seParts) != 2 || !pkg.IsValidRandomID(seParts[1]) {
			// skip, ignore sidecar logs (like route53-sidecar or fluentbit)
			return nil
		}

		service, etag := seParts[0], seParts[1]
		header.Etag = etag
		header.Service = service
		header.Mode = SSModeAwslogs

		return &header
	}

	if strings.Contains(*event.LogStreamName, "-firelens-") {
		// These events are from the Firelens sidecar; try to parse the JSON
		var record logs.FirelensMessage
		if err := json.Unmarshal([]byte(*event.Message), &record); err == nil {
			header.Etag = record.Etag
			header.Host = record.Host             // TODO: use "kaniko" for kaniko logs
			header.Service = record.ContainerName // TODO: could be service_etag
			header.Mode = SSModeFirelens
		}

		return &header
	}

	if strings.HasSuffix(*event.LogGroupIdentifier, "/ecs") || strings.HasSuffix(*event.LogGroupIdentifier, "/ecs:*") {
		header.Etag = bs.etag
		header.Service = "ecs"
		header.Mode = SSModeECS

		return &header
	}

	return nil
}

func (bs *byocServerStream) makeResponse(header *TailResponseHeader, events []ecs.LogEvent) *defangv1.TailResponse {
	entries := make([]*defangv1.LogEntry, 0, len(events))
	response := &defangv1.TailResponse{
		Service: header.Service,
		Host:    header.Host,
		Etag:    header.Etag,
		Entries: entries,
	}

	for _, event := range events {
		entry := bs.makeEntry(header.Mode, event)
		response.Entries = append(response.Entries, entry)
	}

	return response
}

func (bs *byocServerStream) makeEntry(mode SSMode, event ecs.LogEvent) *defangv1.LogEntry {
	entry := defangv1.LogEntry{
		Message:   *event.Message,
		Stderr:    false,
		Timestamp: timestamppb.New(time.UnixMilli(*event.Timestamp)),
	}

	switch mode {
	case SSModeAwslogs:
		var record logs.FirelensMessage
		err := json.Unmarshal([]byte(entry.Message), &record)
		if err != nil {
			term.Debugf("error parsing Firelens message, output raw event log: %v", err)
		} else {
			entry.Message = record.Log
			if record.ContainerName == "kaniko" {
				entry.Stderr = logs.IsLogrusError(entry.Message)
			} else {
				entry.Stderr = record.Source == logs.SourceStderr
			}
		}
	case SSModeECS:
		evt, err := ecs.ParseECSEvent([]byte(*event.Message))
		if err != nil {
			term.Debugf("error parsing ECS event, output raw event log: %v", err)
		} else {
			bs.ecsEventsHandler.HandleECSEvent(evt)
			entry.Service = evt.Service()
			entry.Etag = evt.Etag()
			entry.Host = evt.Host()
			entry.Message = evt.Status()
		}
	case SSModeCd:
		if strings.HasPrefix(entry.Message, logs.ErrorPrefix) {
			entry.Stderr = true
		}
	}

	return &entry
}
