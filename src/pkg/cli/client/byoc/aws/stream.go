package aws

import (
	"context"
	"encoding/json"
	"errors"
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

// byocServerStream is a wrapper around awsecs.EventStream that implements connect-like ServerStream
type byocServerStream struct {
	ctx      context.Context
	err      error
	etag     string
	response *defangv1.TailResponse
	services []string
	stream   ecs.EventStream

	ecsEventsHandler ECSEventHandler
	cdTaskFailureCh  chan cdTaskFailureEvent
}

type cdTaskFailureEvent struct {
	etag      string
	cdTaskArn ecs.TaskArn
	err       ecs.TaskFailure
}

func (e cdTaskFailureEvent) Service() string { return "cd" }
func (e cdTaskFailureEvent) Etag() string    { return e.etag }
func (e cdTaskFailureEvent) Host() string    { return *e.cdTaskArn }
func (e cdTaskFailureEvent) Status() string  { return e.err.Error() }
func (e cdTaskFailureEvent) State() defangv1.ServiceState {
	// CD task failure indicates a deployment failure
	return defangv1.ServiceState_DEPLOYMENT_FAILED
}

func newByocServerStream(ctx context.Context, stream ecs.EventStream, etag string, services []string, ecsEventHandler ECSEventHandler) *byocServerStream {

	bss := &byocServerStream{
		cdTaskFailureCh: make(chan cdTaskFailureEvent),
		ctx:             ctx,
		etag:            etag,
		stream:          stream,
		services:        services,

		ecsEventsHandler: ecsEventHandler,
	}

	if cdTaskArnProvider, ok := ecsEventHandler.(CDTaskArnProvider); ok {
		cdTaskArn := cdTaskArnProvider.GetCDTaskArn(etag)
		if cdTaskArn != nil {
			go func() {
				var taskFailure ecs.TaskFailure
				err := ecs.WaitForTask(ctx, cdTaskArn, time.Second*3)
				term.Debugf("CD task %s has stopped: %v", *cdTaskArn, err)
				if err != nil && errors.As(err, &taskFailure) {
					bss.cdTaskFailureCh <- cdTaskFailureEvent{etag: etag, cdTaskArn: cdTaskArn, err: taskFailure}
				}
				// Ignore other cd errors
			}()
		}
	}

	return bss
}

var _ client.ServerStream[defangv1.TailResponse] = (*byocServerStream)(nil)

func (bs *byocServerStream) Close() error {
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

func (bs *byocServerStream) Receive() bool {
	var evts []ecs.LogEvent
	select {
	case e := <-bs.stream.Events(): // blocking
		if bs.stream.Err() != nil {
			bs.err = bs.stream.Err()
			return false
		}
		var err error
		evts, err = ecs.GetLogEvents(e)
		if err != nil {
			bs.err = err
			return false
		}
	case taskFailure := <-bs.cdTaskFailureCh:
		bs.ecsEventsHandler.HandleECSEvent(taskFailure)
		return true // continue to receive other events, subscribe will handle the failure
	case <-bs.ctx.Done(): // blocking (if not nil)
		bs.err = context.Cause(bs.ctx)
		return false
	}

	bs.response, bs.err = bs.parseEvents(evts)
	if bs.err != nil {
		return false
	}
	return true
}

func (bs *byocServerStream) parseEvents(events []ecs.LogEvent) (*defangv1.TailResponse, error) {
	var response defangv1.TailResponse
	if len(events) == 0 {
		// The original gRPC/connect server stream would never send an empty response.
		// We could loop around the select, but returning an empty response updates the spinner.
		return nil, nil
	}
	parseFirelensRecords := false
	parseECSEventRecords := false
	// Get the Etag/Host/Service from the first event (should be the same for all events in this batch)
	event := events[0]
	if parts := strings.Split(*event.LogStreamName, "/"); len(parts) == 3 {
		if strings.Contains(*event.LogGroupIdentifier, ":"+byoc.CdTaskPrefix) {
			// These events are from the CD task: "crun/main/taskID" stream; we should detect stdout/stderr
			response.Etag = bs.etag // pass the etag filter below, but we already filtered the tail by taskID
			response.Host = "pulumi"
			response.Service = "cd"
		} else {
			// These events are from an awslogs service task: "tenant/service_etag/taskID" stream
			response.Host = parts[2] // TODO: figure out actual hostname/IP
			parts = strings.Split(parts[1], "_")
			if len(parts) != 2 || !pkg.IsValidRandomID(parts[1]) {
				// skip, ignore sidecar logs (like route53-sidecar or fluentbit)
				return nil, nil
			}
			service, etag := parts[0], parts[1]
			response.Etag = etag
			response.Service = service
		}
	} else if strings.Contains(*event.LogStreamName, "-firelens-") {
		// These events are from the Firelens sidecar; try to parse the JSON
		var record logs.FirelensMessage
		if err := json.Unmarshal([]byte(*event.Message), &record); err == nil {
			response.Etag = record.Etag
			response.Host = record.Host             // TODO: use "kaniko" for kaniko logs
			response.Service = record.ContainerName // TODO: could be service_etag
			parseFirelensRecords = true
		}
	} else if strings.HasSuffix(*event.LogGroupIdentifier, "/ecs") || strings.HasSuffix(*event.LogGroupIdentifier, "/ecs:*") {
		parseECSEventRecords = true
		response.Etag = bs.etag
		response.Service = "ecs"
	}

	// Client-side filtering
	if bs.etag != "" && bs.etag != response.Etag {
		return nil, nil // TODO: filter these out using the AWS StartLiveTail API
	}

	if len(bs.services) > 0 && !pkg.Contains(bs.services, response.GetService()) {
		return nil, nil // TODO: filter these out using the AWS StartLiveTail API
	}

	entries := make([]*defangv1.LogEntry, 0, len(events))
	for _, event := range events {
		entry := &defangv1.LogEntry{
			Message:   *event.Message,
			Stderr:    false,
			Timestamp: timestamppb.New(time.UnixMilli(*event.Timestamp)),
		}
		if parseFirelensRecords {
			var record logs.FirelensMessage
			if err := json.Unmarshal([]byte(entry.Message), &record); err == nil {
				entry.Message = record.Log
				if record.ContainerName == "kaniko" {
					entry.Stderr = logs.IsLogrusError(entry.Message)
				} else {
					entry.Stderr = record.Source == logs.SourceStderr
				}
			}
		} else if parseECSEventRecords {
			var err error
			if err = bs.parseECSEventRecord(event, entry); err != nil {
				term.Debugf("error parsing ECS event, output raw event log: %v", err)
			}
		} else if response.Service == "cd" && strings.HasPrefix(entry.Message, " ** ") {
			entry.Stderr = true
		}
		if entry.Etag != "" && bs.etag != "" && entry.Etag != bs.etag {
			continue
		}
		if entry.Service != "" && len(bs.services) > 0 && !pkg.Contains(bs.services, entry.Service) {
			continue
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, nil
	}
	response.Entries = entries
	return &response, nil
}

func (bs *byocServerStream) parseECSEventRecord(event ecs.LogEvent, entry *defangv1.LogEntry) error {
	evt, err := ecs.ParseECSEvent([]byte(*event.Message))
	if err != nil {
		return err
	}
	bs.ecsEventsHandler.HandleECSEvent(evt)
	entry.Service = evt.Service()
	entry.Etag = evt.Etag()
	entry.Host = evt.Host()
	entry.Message = evt.Status()
	return nil
}
