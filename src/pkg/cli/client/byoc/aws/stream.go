package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/logs"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// byocServerStream is a wrapper around awsecs.EventStream that implements connect-like ServerStream
type byocServerStream struct {
	ctx      context.Context
	err      error
	errCh    <-chan error
	etag     string
	response *defangv1.TailResponse
	service  string
	stream   ecs.EventStream

	remaining []ecs.LogEvent
}

func newByocServerStream(ctx context.Context, stream ecs.EventStream, etag, service string) *byocServerStream {
	var errCh <-chan error
	if errch, ok := stream.(hasErrCh); ok {
		errCh = errch.Errs()
	}

	return &byocServerStream{
		ctx:     ctx,
		errCh:   errCh,
		etag:    etag,
		stream:  stream,
		service: service,
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
	return annotateAwsError(bs.err)
}

func (bs *byocServerStream) Msg() *defangv1.TailResponse {
	return bs.response
}

type hasErrCh interface {
	Errs() <-chan error
}

func (bs *byocServerStream) Receive() bool {
	var evts = bs.remaining
	if len(evts) == 0 {
		select {
		case e := <-bs.stream.Events(): // blocking
			var err error
			evts, err = ecs.GetLogEvents(e)
			if err != nil {
				bs.err = err
				return false
			}
		case err := <-bs.errCh: // blocking (if not nil)
			bs.err = err
			return false // abort on first error?

		case <-bs.ctx.Done(): // blocking (if not nil)
			bs.err = context.Cause(bs.ctx)
			return false
		}
	}
	entries, remaining, err := bs.parseEvents(evts)
	if err != nil {
		bs.err = err
		return false
	}
	bs.response.Entries = entries
	bs.remaining = remaining
	return true
}

func (bs *byocServerStream) parseEvents(events []ecs.LogEvent) ([]*defangv1.LogEntry, []ecs.LogEvent, error) {
	bs.response = &defangv1.TailResponse{}
	if len(events) == 0 {
		// The original gRPC/connect server stream would never send an empty response.
		// We could loop around the select, but returning an empty response updates the spinner.
		return nil, nil, nil
	}
	var record logs.FirelensMessage
	parseFirelensRecords := false
	parseECSEventRecords := false
	// Get the Etag/Host/Service from the first event (should be the same for all events in this batch)
	event := events[0]
	if parts := strings.Split(*event.LogStreamName, "/"); len(parts) == 3 {
		if strings.Contains(*event.LogGroupIdentifier, ":"+byoc.CdTaskPrefix) {
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
				return nil, nil, nil
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
	} else if strings.HasSuffix(*event.LogGroupIdentifier, "/ecs") || strings.HasSuffix(*event.LogGroupIdentifier, "/ecs:*") {
		var ecsEvt ecs.Event
		if err := json.Unmarshal([]byte(*event.Message), &ecsEvt); err != nil {
			return nil, events[1:], fmt.Errorf("error unmarshaling ECS Event: %w", err)
		}
		parseECSEventRecords = true
		switch ecsEvt.DetailType {
		case "ECS Task State Change":
			var detail ecs.ECSTaskStateChange
			if err := json.Unmarshal(ecsEvt.Detail, &detail); err != nil {
				return nil, events[1:], fmt.Errorf("error unmarshaling ECS task state change: %w", err)
			}
			if len(detail.Containers) < 1 {
				return nil, events[1:], fmt.Errorf("error parsing ECS task state change: missing containers section")
			}
			i := strings.LastIndex(detail.Containers[0].Name, "_")
			if i < 0 {
				return nil, events[1:], fmt.Errorf("error parsing ECS task state change: invalid container name %q", detail.Containers[0].Name)
			}
			bs.response.Etag = detail.Containers[0].Name[i+1:]
			bs.response.Service = detail.Containers[0].Name[:i]
			bs.response.Host = path.Base(ecsEvt.Resources[0])
		case "ECS Service Action", "ECS Deployment State Change": // pretty much the same JSON structure for both
			var detail ecs.ECSDeploymentStateChange
			if err := json.Unmarshal(ecsEvt.Detail, &detail); err != nil {
				return nil, events[1:], fmt.Errorf("error unmarshaling ECS service/deployment event: %v", err)
			}
			ecsSvcName := path.Base(ecsEvt.Resources[0])
			// TODO: etag is not available at service and deployment level, find a possible correlation, possibly task definition revision using the deploymentId
			// bs.response.Etag =
			snStart := strings.LastIndex(ecsSvcName, "_") // ecsSvcName is in the format "project_service-random", our validation does not allow '_' in service names
			snEnd := strings.LastIndex(ecsSvcName, "-")
			if snStart < 0 || snEnd < 0 {
				return nil, events[1:], fmt.Errorf("error parsing ECS service action: invalid service name %q", ecsEvt.Resources[0])
			}
			bs.response.Service = ecsSvcName[snStart+1 : snEnd]
			bs.response.Host = detail.DeploymentId

		default:
			bs.response.Etag = bs.etag // TODO: Is it possible to filter by etag?
			bs.response.Service = "ecs"
			bs.response.Host = path.Base(ecsEvt.Resources[0]) // TODO: Verify this is the service name
		}
	}

	if bs.etag != "" && bs.etag != bs.response.Etag {
		return nil, nil, nil // TODO: filter these out using the AWS StartLiveTail API
	}
	if bs.service != "" && bs.service != bs.response.Service {
		return nil, nil, nil // TODO: filter these out using the AWS StartLiveTail API
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
		} else if parseECSEventRecords {
			var err error
			if message, err = parseECSEventRecord(event); err != nil {
				return nil, events[1:], err
			}
			return []*defangv1.LogEntry{{
				Message:   message,
				Stderr:    stderr,
				Timestamp: timestamppb.New(time.UnixMilli(*event.Timestamp)),
			}}, events[1:], nil
		} else if bs.response.Service == "cd" && strings.HasPrefix(message, " ** ") {
			stderr = true
		}
		entries[i] = &defangv1.LogEntry{
			Message:   message,
			Stderr:    stderr,
			Timestamp: timestamppb.New(time.UnixMilli(*event.Timestamp)),
		}
	}
	return entries, nil, nil
}

func parseECSEventRecord(event ecs.LogEvent) (string, error) {
	var ecsEvt ecs.Event
	if err := json.Unmarshal([]byte(*event.Message), &ecsEvt); err != nil {
		return "", fmt.Errorf("error unmarshaling ECS event: %w", err)
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, "%s %s ", ecsEvt.DetailType, path.Base(ecsEvt.Resources[0]))
	switch ecsEvt.DetailType {
	case "ECS Task State Change":
		var detail ecs.ECSTaskStateChange
		if err := json.Unmarshal(ecsEvt.Detail, &detail); err != nil {
			return "", fmt.Errorf("error unmarshaling ECS task state change: %w", err)
		}
		fmt.Fprintf(&buf, "%s %s", path.Base(detail.ClusterArn), detail.LastStatus)
		if detail.StoppedReason != "" {
			fmt.Fprintf(&buf, " : %s", detail.StoppedReason)
		}

	case "ECS Service Action", "ECS Deployment State Change": // pretty much the same JSON structure for both
		var detail ecs.ECSDeploymentStateChange
		if err := json.Unmarshal(ecsEvt.Detail, &detail); err != nil {
			return "", fmt.Errorf("error unmarshaling ECS service/deployment event: %v", err)
		}
		fmt.Fprintf(&buf, "%s", detail.EventName)
		if detail.Reason != "" {
			fmt.Fprintf(&buf, " : %s", detail.Reason)
		}
		// raw, err := json.MarshalIndent(ecsEvt, "", "  ")
		// if err == nil {
		// 	log.Printf("ECS Event: %s\n", raw)
		// }
	default:
		raw, err := json.MarshalIndent(ecsEvt.Detail, "", "  ")
		if err != nil {
			return "", fmt.Errorf("error marshaling ECS event detail: %w", err)
		}
		fmt.Fprintf(&buf, "\n%s", raw)
	}
	return buf.String(), nil
}
