package gcp

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	auditpb "google.golang.org/genproto/googleapis/cloud/audit"
	"google.golang.org/protobuf/types/known/structpb"
)

type LogParser[T any] func(*loggingpb.LogEntry) (T, error)

type ServerStream[T any] struct {
	ctx     context.Context
	gcp     *gcp.Gcp
	parse   LogParser[T]
	tailers []*gcp.Tailer

	lastResp T
	lastErr  error
	respCh   chan T
	errCh    chan error
	cancel   func()
}

func NewStream[T any](ctx context.Context, gcp *gcp.Gcp, parse LogParser[T]) *ServerStream[T] {
	streamCtx, cancel := context.WithCancel(ctx)
	return &ServerStream[T]{
		ctx:   streamCtx,
		gcp:   gcp,
		parse: parse,

		respCh: make(chan T),
		errCh:  make(chan error),
		cancel: cancel,
	}
}

func (s *ServerStream[T]) Close() error {
	for _, t := range s.tailers {
		if err := t.Close(); err != nil {
			return err
		}
	}
	s.cancel() // TODO: investigate if we need to close the tailer
	return nil
}

func (s *ServerStream[T]) Receive() bool {
	select {
	case s.lastResp = <-s.respCh:
		return true
	case s.lastErr = <-s.errCh:
		return false
	}
}

func (s *ServerStream[T]) AddTailer(t *gcp.Tailer) {
	s.tailers = append(s.tailers, t)
	go func() {
		for {
			entry, err := t.Next(s.ctx)
			if err != nil {
				s.errCh <- err
				return
			}
			resp, err := s.parse(entry)
			if err != nil {
				s.errCh <- err
				return
			}
			s.respCh <- resp
		}
	}()
}

func (s *ServerStream[T]) Err() error {
	return s.lastErr
}

func (s *ServerStream[T]) Msg() T {
	return s.lastResp
}

type LogStream struct {
	*ServerStream[*defangv1.TailResponse]
}

func NewLogStream(ctx context.Context, gcp *gcp.Gcp) *LogStream {
	return &LogStream{
		ServerStream: NewStream(ctx, gcp, getLogEntryParser(ctx, gcp)),
	}
}

func (s *LogStream) AddJobLog(ctx context.Context, project, executionName string, services []string, since time.Time) error {
	tailer, err := s.gcp.NewTailer(ctx)
	if err != nil {
		return err
	}
	if err := tailer.AddJobLog(ctx, project, executionName, services, since); err != nil {
		return err
	}
	s.ServerStream.AddTailer(tailer)
	return nil
}

func (s *LogStream) AddServiceLog(ctx context.Context, project, etag string, services []string, since time.Time) error {
	tailer, err := s.gcp.NewTailer(ctx)
	if err != nil {
		return err
	}
	if err := tailer.AddServiceLog(ctx, project, etag, services, since); err != nil {
		return err
	}
	s.ServerStream.AddTailer(tailer)
	return nil
}

type SubscribeStream struct {
	*ServerStream[*defangv1.SubscribeResponse]
}

func NewSubscribeStream(ctx context.Context, gcp *gcp.Gcp) *SubscribeStream {
	return &SubscribeStream{
		ServerStream: NewStream(ctx, gcp, ParseActivityEntry),
	}
}

func (s *SubscribeStream) AddJobExecutionUpdate(ctx context.Context, executionName string) error {
	tailer, err := s.gcp.NewTailer(ctx)
	if err != nil {
		return err
	}
	if err := tailer.AddJobExecutionUpdate(ctx, executionName); err != nil {
		return err
	}
	s.ServerStream.AddTailer(tailer)
	return nil
}

func (s *SubscribeStream) AddServiceStatusUpdate(ctx context.Context, project, etag string, services []string) error {
	tailer, err := s.gcp.NewTailer(ctx)
	if err != nil {
		return err
	}
	if err := tailer.AddServiceStatusUpdate(ctx, project, etag, services); err != nil {
		return err
	}
	s.ServerStream.AddTailer(tailer)
	return nil
}

var cdExecutionNamePattern = regexp.MustCompile(`^defang-cd-[a-z0-9]{5}$`)

func getLogEntryParser(ctx context.Context, gcp *gcp.Gcp) func(entry *loggingpb.LogEntry) (*defangv1.TailResponse, error) {
	envCache := make(map[string]map[string]string)

	return func(entry *loggingpb.LogEntry) (*defangv1.TailResponse, error) {
		if entry == nil {
			return nil, nil
		}

		var serviceName, etag, host string
		serviceName = entry.Labels["defang-service"]
		// Log from service
		if serviceName != "" {
			etag = entry.Labels["defang-etag"]
			host = entry.Labels["instanceId"]
			if len(host) > 8 {
				host = host[:8]
			}
		} else {
			executionName := entry.Labels["run.googleapis.com/execution_name"]
			if executionName == "" {
				fmt.Printf("Entry: %+v\n", entry)
				return nil, errors.New("missing both execution name and defang-service in log entry")
			}
			env, ok := envCache[executionName]
			if !ok {
				var err error
				env, err = gcp.GetExecutionEnv(ctx, executionName)
				if err != nil {
					return nil, err
				}
				envCache[executionName] = env
			}

			if cdExecutionNamePattern.MatchString(executionName) { // Special CD case
				serviceName = "cd"
			} else {
				serviceName = env["DEFANG_SERVICE"]
			}

			etag = env["DEFANG_ETAG"]
			host = executionName
		}

		return &defangv1.TailResponse{
			Service: serviceName,
			Etag:    etag,
			Entries: []*defangv1.LogEntry{
				{
					Message:   entry.GetTextPayload(),
					Timestamp: entry.Timestamp,
					Etag:      etag,
					Service:   serviceName,
					Host:      host,
					Stderr:    strings.HasSuffix(entry.LogName, "run.googleapis.com%2Fstderr"),
				},
			},
		}, nil
	}
}

func ParseActivityEntry(entry *loggingpb.LogEntry) (*defangv1.SubscribeResponse, error) {
	if entry == nil {
		return nil, nil
	}

	if entry.GetProtoPayload().GetTypeUrl() != "type.googleapis.com/google.cloud.audit.AuditLog" {
		return nil, errors.New("unexpected log entry type : " + entry.GetProtoPayload().GetTypeUrl())
	}

	auditLog := new(auditpb.AuditLog)
	if err := entry.GetProtoPayload().UnmarshalTo(auditLog); err != nil {
		panic("failed to unmarshal audit log : " + err.Error())
	}

	switch entry.Resource.Type {
	case "cloud_run_revision": // Service status update
		serviceName := path.Base(auditLog.GetResourceName())
		if serviceName == "" {
			return nil, errors.New("missing resource name in audit log")
		}
		if len(serviceName) <= 8 {
			return nil, errors.New("Invalid resource name in audit log : " + serviceName)
		}
		serviceName = serviceName[:len(serviceName)-8] // Remove the random suffix
		// etag is at protoPayload.response.spec.template.metadata.labels.defang-etag
		// etag := getValueInStruct(auditLog.GetResponse(), "spec.template.metadata.labels.defang-etag") // etag not needed
		status := auditLog.GetStatus()
		if status == nil {
			return nil, errors.New("missing status in audit log")
		}
		var state defangv1.ServiceState
		if status.GetCode() == 0 {
			state = defangv1.ServiceState_DEPLOYMENT_COMPLETED
		} else {
			state = defangv1.ServiceState_DEPLOYMENT_FAILED
		}

		return &defangv1.SubscribeResponse{
			Name:   serviceName,
			State:  state,
			Status: status.GetMessage(),
		}, nil

	case "cloud_run_job": // Job execution update
		executionName := path.Base(auditLog.GetResourceName())
		if executionName == "" {
			return nil, errors.New("missing resource name in audit log")
		}
		if len(executionName) <= 6 {
			return nil, errors.New("Invalid resource name in audit log : " + executionName)
		}
		executionName = executionName[:len(executionName)-6] // Remove the random suffix
		if executionName == "defang-cd" && auditLog.GetStatus().GetCode() != 0 {
			return nil, errors.New("defang CD task failed: " + auditLog.GetStatus().GetMessage())
		}
		// TODO: Handle kaniko build task status
		return nil, nil // Ignore success cd status
	default:
		return nil, errors.New("unexpected resource type : " + entry.Resource.Type)
	}
}

// Extract a string value from a nested structpb.Struct
func GetValueInStruct(s *structpb.Struct, path string) string {
	keys := strings.Split(path, ".")
	for len(keys) > 0 {
		if s == nil {
			return ""
		}
		key := keys[0]
		field := s.GetFields()[key]
		if s = field.GetStructValue(); s == nil {
			return field.GetStringValue()
		}
		keys = keys[1:]
	}
	return ""
}
