package gcp

import (
	"context"
	"errors"
	"io"
	"path"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	auditpb "google.golang.org/genproto/googleapis/cloud/audit"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type LogParser[T any] func(*loggingpb.LogEntry) ([]T, error)
type LogFilter[T any] func(entry T) bool

type ServerStream[T any] struct {
	ctx     context.Context
	gcp     *gcp.Gcp
	parse   LogParser[T]
	filters []LogFilter[T]
	query   *Query
	tailer  *gcp.Tailer

	lastResp T
	lastErr  error
	respCh   chan T
	errCh    chan error
	cancel   func()
}

func NewServerStream[T any](ctx context.Context, gcp *gcp.Gcp, parse LogParser[T], filters ...LogFilter[T]) (*ServerStream[T], error) {
	tailer, err := gcp.NewTailer(ctx)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	return &ServerStream[T]{
		ctx:     streamCtx,
		gcp:     gcp,
		parse:   parse,
		filters: filters,
		tailer:  tailer,

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

func (s *ServerStream[T]) Start(start time.Time) {
	query := s.query.GetQuery()
	term.Debugf("Query and tail logs since %v with query: \n%v", start, query)
	go func() {
		// Only query older logs if start time is more than 10ms ago
		if !start.IsZero() && start.Unix() > 0 && time.Since(start) > 10*time.Millisecond {
			lister, err := s.gcp.ListLogEntries(s.ctx, query)
			if err != nil {
				s.errCh <- err
				return
			}
			for {
				entry, err := lister.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					s.errCh <- err
					return
				}
				resps, err := s.parseAndFilter(entry)
				if err != nil {
					s.errCh <- err
					return
				}
				for _, resp := range resps {
					s.respCh <- resp
				}
			}
		}

		// Start tailing logs after all older logs are processed
		if err := s.tailer.Start(s.ctx, query); err != nil {
			s.errCh <- err
			return
		}
		for {
			entry, err := s.tailer.Next(s.ctx)
			if err != nil {
				s.errCh <- err
				return
			}
			resps, err := s.parseAndFilter(entry)
			if err != nil {
				s.errCh <- err
				return
			}
			for _, resp := range resps {
				s.respCh <- resp
			}
		}
	}()
}

func (s *ServerStream[T]) parseAndFilter(entry *loggingpb.LogEntry) ([]T, error) {
	resps, err := s.parse(entry)
	if err != nil {
		return nil, err
	}
	newResps := make([]T, 0, len(resps))
	for _, resp := range resps {
		include := true
		for _, admit := range s.filters {
			if !admit(resp) {
				include = false
				break
			}
		}
		if include {
			newResps = append(newResps, resp)
		}
	}
	return newResps, nil
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

	ss.query = NewLogQuery(gcpClient.ProjectId)
	return &LogStream{ServerStream: ss}, nil
}

func (s *LogStream) AddJobExecutionLog(executionName string) {
	s.query.AddJobExecutionQuery(executionName)
}

func (s *LogStream) AddJobLog(stack, project, etag string, services []string) {
	s.query.AddJobLogQuery(stack, project, etag, services)
}

func (s *LogStream) AddServiceLog(stack, project, etag string, services []string) {
	s.query.AddServiceLogQuery(stack, project, etag, services)
	s.query.AddComputeEngineLogQuery(stack, project, etag, services)
}

func (s *LogStream) AddCloudBuildLog(stack, project, etag string, services []string) {
	s.query.AddCloudBuildLogQuery(stack, project, etag, services)
}

func (s *LogStream) AddSince(start time.Time) {
	s.query.AddSince(start)
}

func (s *LogStream) AddUntil(end time.Time) {
	s.query.AddUntil(end)
}

func (s *LogStream) AddFilter(filter string) {
	s.query.AddFilter(filter)
}

func (s *LogStream) AddCustomQuery(query string) {
	s.query.AddQuery(query)
}

type SubscribeStream struct {
	*ServerStream[*defangv1.SubscribeResponse]
}

func NewSubscribeStream(ctx context.Context, gcp *gcp.Gcp, waitForCD bool, filters ...LogFilter[*defangv1.SubscribeResponse]) (*SubscribeStream, error) {
	ss, err := NewServerStream(ctx, gcp, getActivityParser(waitForCD), filters...)
	if err != nil {
		return nil, err
	}
	ss.query = NewSubscribeQuery()
	return &SubscribeStream{ServerStream: ss}, nil
}

func (s *SubscribeStream) AddJobExecutionUpdate(executionName string) {
	s.query.AddJobExecutionUpdateQuery(executionName)
}

func (s *SubscribeStream) AddJobStatusUpdate(stack, project, etag string, services []string) {
	s.query.AddJobStatusUpdateRequestQuery(stack, project, etag, services)
	s.query.AddJobStatusUpdateResponseQuery(stack, project, etag, services)
}

func (s *SubscribeStream) AddServiceStatusUpdate(stack, project, etag string, services []string) {
	s.query.AddServiceStatusRequestUpdate(stack, project, etag, services)
	s.query.AddServiceStatusReponseUpdate(stack, project, etag, services)
	s.query.AddComputeEngineInstanceGroupInsertOrPatch(stack, project, etag, services)
	s.query.AddComputeEngineInstanceGroupAddInstances()
}

func (s *SubscribeStream) AddCustomQuery(query string) {
	s.query.AddQuery(query)
}

var cdExecutionNamePattern = regexp.MustCompile(`^defang-cd-[a-z0-9]{5}$`)

func getLogEntryParser(ctx context.Context, gcp *gcp.Gcp) func(entry *loggingpb.LogEntry) ([]*defangv1.TailResponse, error) {
	envCache := make(map[string]map[string]string)
	return func(entry *loggingpb.LogEntry) ([]*defangv1.TailResponse, error) {
		if entry == nil {
			return nil, nil
		}

		msg := entry.GetTextPayload()
		if msg == "" && entry.GetJsonPayload() != nil {
			msg = entry.GetJsonPayload().GetFields()["message"].GetStringValue()
		}
		var stderr bool
		if entry.LogName != "" {
			stderr = strings.HasSuffix(entry.LogName, "run.googleapis.com%2Fstderr")
		} else if entry.GetJsonPayload() != nil && entry.GetJsonPayload().GetFields()["cos.googleapis.com/stream"] != nil {
			stderr = entry.GetJsonPayload().GetFields()["cos.googleapis.com/stream"].GetStringValue() == "stderr"
		}

		// fmt.Printf("ENTRY: %+v\n", entry)

		var serviceName, etag, host string
		serviceName = entry.Labels["defang-service"]
		executionName := entry.Labels["run.googleapis.com/execution_name"]
		buildTags := entry.Labels["build_tags"]
		// Log from service
		if serviceName != "" {
			etag = entry.Labels["defang-etag"]
			host = entry.Labels["instanceId"] // cloudrun instance
			if host == "" {
				host = entry.Resource.Labels["instance_id"] // compute engine instance
			}
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
			var err error
			_, msg, err = LogEntryToString(entry)
			if err != nil {
				return nil, err
			}
		}

		return []*defangv1.TailResponse{
			{
				Service: serviceName,
				Etag:    etag,
				Entries: []*defangv1.LogEntry{
					{
						Message:   msg,
						Timestamp: entry.Timestamp,
						Etag:      etag,
						Service:   serviceName,
						Host:      host,
						Stderr:    stderr,
					},
				},
			},
		}, nil
	}
}

const defangCD = "#defang-cd" // Special service name for CD, # is used to avoid conflict with service names

func getActivityParser(waitForCD bool) func(entry *loggingpb.LogEntry) ([]*defangv1.SubscribeResponse, error) {
	cdSuccess := false
	readyServices := make(map[string]string)

	computeEngineRootTriggers := make(map[string]string)

	return func(entry *loggingpb.LogEntry) ([]*defangv1.SubscribeResponse, error) {
		if entry == nil {
			return nil, nil
		}

		if entry.GetProtoPayload().GetTypeUrl() != "type.googleapis.com/google.cloud.audit.AuditLog" {
			term.Warnf("unexpected log entry type : %v", entry.GetProtoPayload().GetTypeUrl())
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
					if cdSuccess || !waitForCD {
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
				term.Warnf("missing request and response in audit log for service %v", path.Base(auditLog.GetResourceName()))
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
					term.Warnf("missing status in audit log for job %v", path.Base(auditLog.GetResourceName()))
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
					return nil, client.ErrDeploymentFailed{Message: auditLog.GetStatus().GetMessage()}
				}
				cdSuccess = true
				// Report all ready services when CD is successful, prevents cli deploy stop before cd is done
				resps := make([]*defangv1.SubscribeResponse, 0, len(readyServices))
				for serviceName, status := range readyServices {
					resps = append(resps, &defangv1.SubscribeResponse{
						Name:   serviceName,
						State:  defangv1.ServiceState_DEPLOYMENT_COMPLETED,
						Status: status,
					})
				}
				resps = append(resps, &defangv1.SubscribeResponse{
					Name:   defangCD,
					State:  defangv1.ServiceState_DEPLOYMENT_COMPLETED,
					Status: auditLog.GetStatus().GetMessage(),
				})
				return resps, nil // Ignore success cd status when we are waiting for service status
			} else {
				term.Warnf("unexpected execution name in audit log : %v", executionName)
				return nil, nil
			}
		case "gce_instance_group_manager": // Compute engine update start
			request := auditLog.GetRequest()
			if request == nil {
				term.Warnf("missing request in audit log for instance group manager %v", path.Base(auditLog.GetResourceName()))
				return nil, nil
			}
			labels := GetListInStruct(request, "allInstancesConfig.properties.labels")
			if labels == nil {
				term.Warnf("missing labels in audit log for instance group manager %v", path.Base(auditLog.GetResourceName()))
				return nil, nil
			}
			// Find the service name from the labels
			serviceName := ""
			for _, label := range labels {
				fields := label.GetStructValue().GetFields()
				if fields["key"].GetStringValue() == "defang-service" {
					serviceName = fields["value"].GetStringValue()
					break
				}
			}
			if serviceName == "" {
				term.Warnf("missing defang-service label in audit log for instance group manager %v", path.Base(auditLog.GetResourceName()))
				return nil, nil
			}
			rootTriggerId := entry.GetLabels()["compute.googleapis.com/root_trigger_id"]
			if rootTriggerId == "" {
				term.Warnf("missing root_trigger_id in audit log for instance group manager %v", path.Base(auditLog.GetResourceName()))
			} else {
				computeEngineRootTriggers[rootTriggerId] = serviceName
			}
			return []*defangv1.SubscribeResponse{{
				Name:   serviceName,
				State:  defangv1.ServiceState_DEPLOYMENT_PENDING,
				Status: auditLog.GetResponse().GetFields()["status"].GetStringValue(),
			}}, nil
		case "gce_instance_group": // Compute engine update end
			// TODO: Better handle of multiple instance group insert events for the same service where more than 1 replica is created, all of them would have 100% for progress and DONE as status
			rootTriggerId := entry.GetLabels()["compute.googleapis.com/root_trigger_id"]
			serviceName, ok := computeEngineRootTriggers[rootTriggerId]
			if !ok {
				term.Debugf("ignored root trigger id %v for instance group insert", rootTriggerId)
				return nil, nil
			}
			response := auditLog.GetResponse()
			if response == nil {
				term.Warnf("missing response in audit log for instance group %v", path.Base(auditLog.GetResourceName()))
				return nil, nil
			}
			status := response.GetFields()["status"].GetStringValue()
			var state defangv1.ServiceState
			switch status {
			case "DONE":
				state = defangv1.ServiceState_DEPLOYMENT_COMPLETED
			case "RUNNING":
				state = defangv1.ServiceState_DEPLOYMENT_PENDING
			default:
				state = defangv1.ServiceState_DEPLOYMENT_FAILED
			}
			return []*defangv1.SubscribeResponse{{
				Name:   serviceName,
				State:  state,
				Status: status,
			}}, nil
		// TODO: Add cloud build activities for building status update
		default:
			term.Warnf("unexpected resource type : %v", entry.Resource.Type)
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

func GetListInStruct(s *structpb.Struct, path string) []*structpb.Value {
	keys := strings.Split(path, ".")
	for len(keys) > 0 {
		if s == nil {
			return nil
		}
		key := keys[0]
		field := s.GetFields()[key]
		if s = field.GetStructValue(); s == nil {
			return field.GetListValue().Values
		}
		keys = keys[1:]
	}
	return nil
}
