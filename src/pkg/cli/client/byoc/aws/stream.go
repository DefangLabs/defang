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
	"github.com/DefangLabs/defang/src/pkg/term"
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
	services []string
	stream   ecs.EventStream
}

func newByocServerStream(ctx context.Context, stream ecs.EventStream, etag string, services []string) *byocServerStream {
	var errCh <-chan error
	if errch, ok := stream.(hasErrCh); ok {
		errCh = errch.Errs()
	}

	return &byocServerStream{
		ctx:      ctx,
		errCh:    errCh,
		etag:     etag,
		stream:   stream,
		services: services,
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
	var evts []ecs.LogEvent
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
	}

	// Client-side filtering
	if bs.etag != "" && bs.etag != response.Etag {
		return nil, nil // TODO: filter these out using the AWS StartLiveTail API
	}

	if len(bs.services) > 0 && !pkg.Contains(bs.services, bs.response.GetService()) {
		return nil, nil // TODO: filter these out using the AWS StartLiveTail API
	}

	entries := make([]*defangv1.LogEntry, len(events))
	for i, event := range events {
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
			if err = parseECSEventRecord(event, entry); err != nil {
				term.Debugf("error parsing ECS event, output raw event log: %v", err)
			}
		} else if response.Service == "cd" && strings.HasPrefix(entry.Message, " ** ") {
			entry.Stderr = true
		}
		entries[i] = entry
	}
	response.Entries = entries
	return &response, nil
}

func parseECSEventRecord(event ecs.LogEvent, entry *defangv1.LogEntry) error {
	var ecsEvt ecs.Event
	if err := json.Unmarshal([]byte(*event.Message), &ecsEvt); err != nil {
		return fmt.Errorf("error unmarshaling ECS event: %w", err)
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, "%s ", ecsEvt.DetailType)
	if len(ecsEvt.Resources) > 0 {
		fmt.Fprintf(&buf, "%s ", path.Base(ecsEvt.Resources[0]))
	}
	switch ecsEvt.DetailType {
	case "ECS Task State Change":
		var detail ecs.ECSTaskStateChange
		if err := json.Unmarshal(ecsEvt.Detail, &detail); err != nil {
			return fmt.Errorf("error unmarshaling ECS task state change: %w", err)
		}

		// Container name is in the format of "service_etag"
		if len(detail.Containers) < 1 {
			return fmt.Errorf("error parsing ECS task state change: missing containers section")
		}
		i := strings.LastIndex(detail.Containers[0].Name, "_")
		if i < 0 {
			return fmt.Errorf("error parsing ECS task state change: invalid container name %q", detail.Containers[0].Name)
		}
		entry.Service = detail.Containers[0].Name[:i]
		entry.Etag = detail.Containers[0].Name[i+1:]
		entry.Host = path.Base(ecsEvt.Resources[0])
		fmt.Fprintf(&buf, "%s %s", path.Base(detail.ClusterArn), detail.LastStatus)
		if detail.StoppedReason != "" {
			fmt.Fprintf(&buf, " : %s", detail.StoppedReason)
		}
	case "ECS Service Action", "ECS Deployment State Change": // pretty much the same JSON structure for both
		var detail ecs.ECSDeploymentStateChange
		if err := json.Unmarshal(ecsEvt.Detail, &detail); err != nil {
			return fmt.Errorf("error unmarshaling ECS service/deployment event: %v", err)
		}
		ecsSvcName := path.Base(ecsEvt.Resources[0])
		// TODO: etag is not available at service and deployment level, find a possible correlation, possibly task definition revision using the deploymentId
		snStart := strings.LastIndex(ecsSvcName, "_") // ecsSvcName is in the format "project_service-random", our validation does not allow '_' in service names
		snEnd := strings.LastIndex(ecsSvcName, "-")
		if snStart < 0 || snEnd < 0 || snStart >= snEnd {
			return fmt.Errorf("error parsing ECS service action: invalid service name %q", ecsEvt.Resources[0])
		}
		entry.Service = ecsSvcName[snStart+1 : snEnd]
		entry.Host = detail.DeploymentId
		fmt.Fprintf(&buf, "%s", detail.EventName)
		if detail.Reason != "" {
			fmt.Fprintf(&buf, " : %s", detail.Reason)
		}
	default:
		entry.Service = "ecs"
		if len(ecsEvt.Resources) > 0 {
			entry.Host = path.Base(ecsEvt.Resources[0])
		}
		// Print the unrecogonalized ECS event detail in prettry JSON format if possible
		raw, err := json.MarshalIndent(ecsEvt.Detail, "", "  ")
		if err != nil {
			raw = []byte(ecsEvt.Detail)
		}
		fmt.Fprintf(&buf, "\n%s", raw)
	}
	entry.Message = buf.String()
	return nil
}
