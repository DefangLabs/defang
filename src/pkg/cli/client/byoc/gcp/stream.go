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
	"github.com/DefangLabs/defang/src/pkg/term"
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

func NewLogStream(ctx context.Context, gcpClient *gcp.Gcp) (*LogStream, error) {
	ss, err := NewServerStream(ctx, gcpClient, getLogEntryParser(ctx, gcpClient))
	if err != nil {
		return nil, err
	}

	query := gcp.CreateStdQuery(gcpClient.ProjectId)
	ss.tailer.SetBaseQuery(query)
	return &LogStream{ServerStream: ss}, nil
}

func (s *LogStream) AddJobExecutionLog(executionName string, since time.Time) {
	query := gcp.CreateJobExecutionQuery(executionName, since)
	s.tailer.AddQuerySet(query)
}

func (s *LogStream) AddJobLog(project, etag string, services []string, since time.Time) {
	query := gcp.CreateJobLogQuery(project, etag, services, since)
	s.tailer.AddQuerySet(query)
}

func (s *LogStream) AddServiceLog(project, etag string, services []string, since time.Time) {
	query := gcp.CreateServiceLogQuery(project, etag, services, since)
	s.tailer.AddQuerySet(query)
}

func (s *LogStream) AddCloudBuildLog(project, etag string, services []string, since time.Time) {
	query := gcp.CreateCloudBuildLogQuery(project, etag, services, since)
	s.tailer.AddQuerySet(query)
}

type SubscribeStream struct {
	*ServerStream[*defangv1.SubscribeResponse]
}

func NewSubscribeStream(ctx context.Context, gcp *gcp.Gcp, reportCD bool) (*SubscribeStream, error) {
	ss, err := NewServerStream(ctx, gcp, getActivityParser(reportCD))
	if err != nil {
		return nil, err
	}
	ss.tailer.SetBaseQuery(`protoPayload.serviceName="run.googleapis.com"`)
	return &SubscribeStream{ServerStream: ss}, nil
}

func (s *SubscribeStream) AddJobExecutionUpdate(executionName string) {
	query := gcp.CreateJobExecutionUpdateQuery(executionName)
	s.tailer.AddQuerySet(query)
}

func (s *SubscribeStream) AddJobStatusUpdate(project, etag string, services []string) {
	reqQuery := `protoPayload.methodName="google.cloud.run.v2.Jobs.UpdateJob" OR "google.cloud.run.v2.Jobs.CreateJob"`
	resQuery := `protoPayload.methodName="/Jobs.RunJob" OR "/Jobs.CreateJob" OR "/Jobs.UpdateJob"`

	if project != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.job.template.labels."defang-project"="%v"`, project)
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-project"="%v"`, project)
	}

	if etag != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.job.template.labels."defang-etag"="%v"`, etag)
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		reqQuery += fmt.Sprintf(`
protoPayload.request.job.template.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
	}

	s.tailer.AddQuerySet(fmt.Sprintf("\n(\n%s\n) OR (\n%s\n)", reqQuery, resQuery))
}

func (s *SubscribeStream) AddServiceStatusUpdate(project, etag string, services []string) {
	reqQuery := `protoPayload.methodName="google.cloud.run.v2.Services.CreateService" OR "google.cloud.run.v2.Services.UpdateService"`
	resQuery := `protoPayload.methodName="/Services.CreateService" OR "/Services.UpdateService" OR "/Services.ReplaceService" OR "/Services.DeleteService"`

	if project != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.service.template.labels."defang-service"="%v"`, project)
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-project"="%v"`, project)
	}

	if etag != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.service.template.labels."defang-etag"="%v"`, etag)
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		reqQuery += fmt.Sprintf(`
protoPayload.request.service.template.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
	}
	s.tailer.AddQuerySet(fmt.Sprintf("\n(\n%s\n) OR (\n%s\n)", reqQuery, resQuery))
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
		buildTags := entry.Labels["build_tags"]
		// Log from service
		if serviceName != "" {
			etag = entry.Labels["defang-etag"]
			host = entry.Labels["instanceId"]
			if len(host) > 8 {
				host = host[:8]
			}
			// kaniko build job
			if regexp.MustCompile(`-build-[a-z0-9]{7}-[a-z0-9]{8}$`).MatchString(executionName) {
				serviceName += "-image"
			}
		} else if executionName != "" {
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

			// use kaniko build job environment to get etag
			etag = env["DEFANG_ETAG"]
			host = "pulumi" // Hardcoded to match end condition detector in cmd/cli/command/compose.go
		} else if buildTags != "" {
			parts := strings.Split(buildTags, "_")
			if len(parts) < 3 {
				return nil, errors.New("invalid cloudbuild build tags value: " + buildTags)
			}
			serviceName = parts[len(parts)-2]
			etag = parts[len(parts)-1]
			host = "cloudbuild"
		} else {
			return nil, errors.New("missing both execution name and defang-service in log entry")
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

func getActivityParser(reportCD bool) func(entry *loggingpb.LogEntry) ([]*defangv1.SubscribeResponse, error) {
	cdSuccess := false
	readyServices := make(map[string]string)

	return func(entry *loggingpb.LogEntry) ([]*defangv1.SubscribeResponse, error) {
		if entry == nil {
			return nil, nil
		}

		if entry.GetProtoPayload().GetTypeUrl() != "type.googleapis.com/google.cloud.audit.AuditLog" {
			term.Warn("unexpected log entry type : " + entry.GetProtoPayload().GetTypeUrl())
			return nil, nil
		}

		auditLog := new(auditpb.AuditLog)
		if err := entry.GetProtoPayload().UnmarshalTo(auditLog); err != nil {
			term.Warnf("failed to unmarshal audit log : %v", err)
			return nil, nil
		}

		switch entry.Resource.Type {
		case "cloud_run_revision": // Service status
			if request := auditLog.GetRequest(); request != nil { // Activity log: service update requests
				serviceName := GetValueInStruct(request, "service.template.labels.defang-service")
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
				term.Warn("missing request and response in audit log for service " + path.Base(auditLog.GetResourceName()))
				return nil, nil
			}

			// etag is at protoPayload.response.spec.template.metadata.labels.defang-etag
			// etag := getValueInStruct(auditLog.GetResponse(), "spec.template.metadata.labels.defang-etag") // etag not needed
			// service.spec.template.metadata.labels."defang-service"

		case "cloud_run_job": // Job execution update
			// Kaniko job
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
				serviceName := GetValueInStruct(response, "spec.template.metadata.labels.defang-service")
				status := auditLog.GetStatus()
				if status == nil {
					term.Warn("missing status in audit log for job " + path.Base(auditLog.GetResourceName()))
					return nil, nil
				}
				var state defangv1.ServiceState
				if status.GetCode() == 0 {
					state = defangv1.ServiceState_BUILD_STOPPING
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
			if cdExecutionNamePattern.MatchString(executionName) {
				if auditLog.GetStatus().GetCode() != 0 {
					return nil, pkg.ErrDeploymentFailed{Service: "defang-cd", Message: auditLog.GetStatus().GetMessage()}
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
				if reportCD {
					return []*defangv1.SubscribeResponse{{
						Name:   "defang CD",
						State:  defangv1.ServiceState_DEPLOYMENT_COMPLETED,
						Status: auditLog.GetStatus().GetMessage(),
					}}, nil
				}
				return nil, nil // Ignore success cd status if not reporting cd
			} else {
				term.Warn("unexpected execution name in audit log : " + executionName)
				return nil, nil
			}
		default:
			term.Warn("unexpected resource type : " + entry.Resource.Type)
			return nil, nil
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
