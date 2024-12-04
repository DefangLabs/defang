package gcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	auditpb "google.golang.org/genproto/googleapis/cloud/audit"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type LogParser[T any] func(*loggingpb.LogEntry) ([]T, error)

type ServerStream[T any] struct {
	ctx    context.Context
	gcp    *gcp.Gcp
	parse  LogParser[T]
	tailer *gcp.Tailer

	lastResp T
	lastErr  error
	respCh   chan T
	errCh    chan error
	cancel   func()
}

func NewServerStream[T any](ctx context.Context, gcp *gcp.Gcp, parse LogParser[T]) (*ServerStream[T], error) {
	tailer, err := gcp.NewTailer(ctx)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	return &ServerStream[T]{
		ctx:    streamCtx,
		gcp:    gcp,
		parse:  parse,
		tailer: tailer,

		respCh: make(chan T),
		errCh:  make(chan error),
		cancel: cancel,
	}, nil
}

func (s *ServerStream[T]) Close() error {
	s.cancel() // TODO: investigate if we need to close the tailer
	return nil
}

func (s *ServerStream[T]) Receive() bool {
	select {
	case s.lastResp = <-s.respCh:
		return true
	case err := <-s.errCh:
		if context.Cause(s.ctx) == io.EOF {
			s.lastErr = nil
		} else if isContextCanceledError(err) {
			s.lastErr = context.Cause(s.ctx)
		} else {
			s.lastErr = err
		}
		return false
	}
}

func isContextCanceledError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	if st, ok := status.FromError(err); ok {
		return st.Code() == codes.Canceled
	}
	return false
}

func (s *ServerStream[T]) Start() error {
	if err := s.tailer.Start(s.ctx); err != nil {
		return err
	}
	go func() {
		for {
			entry, err := s.tailer.Next(s.ctx)
			if err != nil {
				s.errCh <- err
				return
			}
			resps, err := s.parse(entry)
			if err != nil {
				s.errCh <- err
				return
			}
			for _, resp := range resps {
				s.respCh <- resp
			}
		}
	}()
	return nil
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

func NewLogStream(ctx context.Context, gcp *gcp.Gcp) (*LogStream, error) {
	ss, err := NewServerStream(ctx, gcp, getLogEntryParser(ctx, gcp))
	if err != nil {
		return nil, err
	}
	return &LogStream{ServerStream: ss}, nil
}

func (s *LogStream) AddJobExecutionLog(executionName string, since time.Time) {
	query := `
resource.type = "cloud_run_job"
logName=~"logs/run.googleapis.com%2F(stdout|stderr)$"`

	query += fmt.Sprintf(`
labels."run.googleapis.com/execution_name" = "%v"`, executionName)

	if !since.IsZero() && since.Unix() > 0 {
		query += fmt.Sprintf(`
timestamp >= "%v"`, since.UTC().Format(time.RFC3339)) // Nano?
	}

	s.tailer.AddQuerySet(query)
}

func (s *LogStream) AddJobLog(project, etag string, services []string, since time.Time) {
	query := `
resource.type = "cloud_run_job"
logName=~"logs/run.googleapis.com%2F(stdout|stderr)$"`

	if project != "" {
		query += fmt.Sprintf(`
labels."defang-project" = "%v"`, project)
	}

	if etag != "" {
		query += fmt.Sprintf(`
labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		query += fmt.Sprintf(`
labels."defang-service" =~ "^(%v)$"`, strings.Join(services, "|"))
	}

	if !since.IsZero() && since.Unix() > 0 {
		query += fmt.Sprintf(`
timestamp >= "%v"`, since.UTC().Format(time.RFC3339)) // Nano?
	}

	s.tailer.AddQuerySet(query)
}

func (s *LogStream) AddServiceLog(project, etag string, services []string, since time.Time) {
	query := `
resource.type="cloud_run_revision"
logName=~"logs/run.googleapis.com%2F(stdout|stderr)$"`

	if etag != "" {
		query += fmt.Sprintf(`
labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		query += fmt.Sprintf(`
labels."defang-service" =~ "^(%v)$"`, strings.Join(services, "|"))
	}

	if !since.IsZero() && since.Unix() > 0 {
		query += fmt.Sprintf(`
timestamp >= "%v"`, since.UTC().Format(time.RFC3339)) // Nano?
	}

	s.tailer.AddQuerySet(query)
}

type SubscribeStream struct {
	*ServerStream[*defangv1.SubscribeResponse]
}

func NewSubscribeStream(ctx context.Context, gcp *gcp.Gcp) (*SubscribeStream, error) {
	ss, err := NewServerStream(ctx, gcp, getActivityParser())
	if err != nil {
		return nil, err
	}
	ss.tailer.SetBaseQuery(`logName:"cloudaudit.googleapis.com" AND protoPayload.serviceName="run.googleapis.com"`)
	return &SubscribeStream{ServerStream: ss}, nil
}

func (s *SubscribeStream) AddJobExecutionUpdate(executionName string) {
	query := fmt.Sprintf(`
labels."run.googleapis.com/execution_name" = "%v"`, executionName)
	s.tailer.AddQuerySet(query)
}

func (s *SubscribeStream) AddJobStatusUpdate(project, etag string, services []string) {
	query := `
protoPayload.methodName="/Jobs.RunJob" OR "/Jobs.CreateJob" OR "google.cloud.run.v2.Jobs.UpdateJob" OR "google.cloud.run.v2.Jobs.CreateJob"`

	if project != "" {
		query += fmt.Sprintf(`
protoPayload.response.metadata.labels."defang-project"="%v"`, project)
	}

	if etag != "" {
		query += fmt.Sprintf(`
protoPayload.response.metadata.labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		query += fmt.Sprintf(`
protoPayload.response.metadata.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
	}

	s.tailer.AddQuerySet(query)
}

func (s *SubscribeStream) AddServiceStatusUpdate(project, etag string, services []string) {
	query := `
protoPayload.methodName="google.cloud.run.v1.Services.CreateService" OR "/Services.CreateService" OR "/Services.ReplaceService" OR "/Services.DeleteService"`

	if project != "" {
		query += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-project"="%v"`, project)
	}

	if etag != "" {
		query += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		query += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
	}
	s.tailer.AddQuerySet(query)
}

var cdExecutionNamePattern = regexp.MustCompile(`^defang-cd-[a-z0-9]{5}$`)

func getLogEntryParser(ctx context.Context, gcp *gcp.Gcp) func(entry *loggingpb.LogEntry) ([]*defangv1.TailResponse, error) {
	envCache := make(map[string]map[string]string)

	return func(entry *loggingpb.LogEntry) ([]*defangv1.TailResponse, error) {
		if entry == nil {
			return nil, nil
		}

		var serviceName, etag, host string
		serviceName = entry.Labels["defang-service"]
		executionName := entry.Labels["run.googleapis.com/execution_name"]
		// Log from service
		if serviceName != "" {
			etag = entry.Labels["defang-etag"]
			host = entry.Labels["instanceId"]
			if len(host) > 8 {
				host = host[:8]
			}
			if regexp.MustCompile(`-build-[a-z0-9]{7}-[a-z0-9]{8}$`).MatchString(executionName) {
				serviceName += "-image"
			}
		} else {
			if executionName == "" {
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
			host = "pulumi" // Hardcoded to match end condition detector in cmd/cli/command/compose.go
		}

		return []*defangv1.TailResponse{
			{
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
			},
		}, nil
	}
}

func getActivityParser() func(entry *loggingpb.LogEntry) ([]*defangv1.SubscribeResponse, error) {
	cdSuccess := false
	readyServices := make(map[string]string)

	return func(entry *loggingpb.LogEntry) ([]*defangv1.SubscribeResponse, error) {
		if entry == nil {
			return nil, nil
		}

		if entry.GetProtoPayload().GetTypeUrl() != "type.googleapis.com/google.cloud.audit.AuditLog" {
			return nil, errors.New("unexpected log entry type : " + entry.GetProtoPayload().GetTypeUrl())
		}

		auditLog := new(auditpb.AuditLog)
		if err := entry.GetProtoPayload().UnmarshalTo(auditLog); err != nil {
			return nil, fmt.Errorf("failed to unmarshal audit log : %w", err)
		}

		switch entry.Resource.Type {
		case "cloud_run_revision": // Service status
			if request := auditLog.GetRequest(); request != nil { // Activity log: service update requests
				serviceName := GetValueInStruct(request, "service.spec.template.metadata.labels.defang-service")
				return []*defangv1.SubscribeResponse{{
					Name:   serviceName,
					State:  defangv1.ServiceState_DEPLOYMENT_PENDING,
					Status: GetValueInStruct(request, "methodName"),
				}}, nil
			} else if response := auditLog.GetResponse(); response != nil { // System log: service status update
				serviceName := GetValueInStruct(response, "spec.template.metadata.labels.defang-service")
				status := auditLog.GetStatus()
				if status == nil {
					return nil, errors.New("missing status in audit log for service " + serviceName)
				}
				var state defangv1.ServiceState
				if status.GetCode() == 0 {
					if cdSuccess {
						state = defangv1.ServiceState_DEPLOYMENT_COMPLETED
					} else {
						state = defangv1.ServiceState_DEPLOYMENT_PENDING // Report later
						readyServices[serviceName] = status.GetMessage()
					}
				} else {
					state = defangv1.ServiceState_DEPLOYMENT_FAILED
				}
				return []*defangv1.SubscribeResponse{{
					Name:   serviceName,
					State:  state,
					Status: status.GetMessage(),
				}}, nil
			} else {
				return nil, errors.New("missing request and response in audit log for service " + path.Base(auditLog.GetResourceName()))
			}

			// etag is at protoPayload.response.spec.template.metadata.labels.defang-etag
			// etag := getValueInStruct(auditLog.GetResponse(), "spec.template.metadata.labels.defang-etag") // etag not needed
			// service.spec.template.metadata.labels."defang-service"

		case "cloud_run_job": // Job execution update
			if request := auditLog.GetRequest(); request != nil { // Acitivity log: job creation
				serviceName := GetValueInStruct(request, "job.template.labels.defang-service")
				if serviceName != "" {
					return []*defangv1.SubscribeResponse{{
						Name:   serviceName,
						State:  defangv1.ServiceState_BUILD_ACTIVATING,
						Status: "Building job creating",
					}}, nil
				}
			} else if response := auditLog.GetResponse(); response != nil { // System log: job status update
				serviceName := GetValueInStruct(response, "metadata.labels.defang-service")
				status := auditLog.GetStatus()
				if status == nil {
					return nil, errors.New("missing status in audit log for job " + path.Base(auditLog.GetResourceName()))
				}
				var state defangv1.ServiceState
				if status.GetCode() == 0 {
					if strings.Contains(status.GetMessage(), "Ready condition status changed to True") { // TODO: Is it better to scan though the conditions instead?
						state = defangv1.ServiceState_BUILD_RUNNING
					} else {
						state = defangv1.ServiceState_BUILD_STOPPING
					}
				} else {
					state = defangv1.ServiceState_DEPLOYMENT_FAILED
				}
				if serviceName != "" {
					return []*defangv1.SubscribeResponse{{
						Name:   serviceName,
						State:  state,
						Status: status.GetMessage(),
					}}, nil
				}
			}
			// CD job
			executionName := path.Base(auditLog.GetResourceName())
			if executionName == "" {
				return nil, errors.New("missing resource name in audit log")
			}
			if len(executionName) <= 6 {
				return nil, errors.New("Invalid resource name in audit log : " + executionName)
			}
			executionName = executionName[:len(executionName)-6] // Remove the random suffix
			if executionName == "defang-cd" {
				if auditLog.GetStatus().GetCode() != 0 {
					return nil, pkg.ErrDeploymentFailed{Service: "defang CD", Message: auditLog.GetStatus().GetMessage()}
				}
				cdSuccess = true
				if len(readyServices) > 0 {
					resps := make([]*defangv1.SubscribeResponse, 0, len(readyServices))
					for serviceName, status := range readyServices {
						resps = append(resps, &defangv1.SubscribeResponse{
							Name:   serviceName,
							State:  defangv1.ServiceState_DEPLOYMENT_COMPLETED,
							Status: status,
						})
					}
					return resps, nil // Ignore success cd status when we are waiting for service status
				}
				return []*defangv1.SubscribeResponse{{
					Name:   "defang CD",
					State:  defangv1.ServiceState_DEPLOYMENT_COMPLETED,
					Status: auditLog.GetStatus().GetMessage(),
				}}, nil
			} else {
				return nil, errors.New("unexpected execution name in audit log : " + executionName)
			}
		default:
			return nil, errors.New("unexpected resource type : " + entry.Resource.Type)
		}
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
