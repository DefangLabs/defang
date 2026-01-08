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
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	auditpb "google.golang.org/genproto/googleapis/cloud/audit"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	logtype "google.golang.org/genproto/googleapis/logging/type"
)

type LogParser[T any] func(*loggingpb.LogEntry) ([]*T, error)
type LogFilter[T any] func(entry T) T

type GcpLogsClient interface {
	ListLogEntries(ctx context.Context, query string, order gcp.Order) (gcp.Lister, error)
	NewTailer(ctx context.Context) (gcp.Tailer, error)
	GetExecutionEnv(ctx context.Context, executionName string) (map[string]string, error)
	GetProjectID() gcp.ProjectId
	GetBuildInfo(ctx context.Context, buildId string) (*gcp.BuildTag, error)
}

type ServerStream[T any] struct {
	ctx           context.Context
	gcpLogsClient GcpLogsClient
	parse         LogParser[T]
	filters       []LogFilter[*T]
	query         *Query
	tailer        gcp.Tailer

	lastResp *T
	lastErr  error
	respCh   chan *T
	errCh    chan error
	cancel   func()
}

func NewServerStream[T any](ctx context.Context, gcpLogsClient GcpLogsClient, parse LogParser[T], filters ...LogFilter[*T]) (*ServerStream[T], error) {
	tailer, err := gcpLogsClient.NewTailer(ctx)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	return &ServerStream[T]{
		ctx:           streamCtx,
		gcpLogsClient: gcpLogsClient,
		parse:         parse,
		filters:       filters,
		tailer:        tailer,

		respCh: make(chan *T),
		errCh:  make(chan error),
		cancel: cancel,
	}, nil
}

func (s *ServerStream[T]) Close() error {
	s.cancel()
	s.tailer.Close() // Close the grpc connection
	return nil
}

func (s *ServerStream[T]) Receive() bool {
	select {
	case s.lastResp = <-s.respCh:
		return true
	case err := <-s.errCh:
		if context.Cause(s.ctx) == io.EOF {
			s.lastErr = nil
		} else if errors.Is(err, io.EOF) {
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

func (s *ServerStream[T]) StartFollow(start time.Time) {
	query := s.query.GetQuery()
	term.Debugf("Query and tail logs since %v with query: \n%v", start, query)
	go func() {
		// Only query older logs if start time is more than 10ms ago
		if !start.IsZero() && start.Unix() > 0 && time.Since(start) > 10*time.Millisecond {
			s.queryHead(query, 0)
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

func (s *ServerStream[T]) StartHead(limit int32) {
	query := s.query.GetQuery()
	term.Debugf("Query logs with query: \n%v", query)
	go func() {
		s.queryHead(query, limit)
	}()
}

func (s *ServerStream[T]) StartTail(limit int32) {
	query := s.query.GetQuery()
	term.Debugf("Query logs with query: \n%v", query)
	go func() {
		s.queryTail(query, limit)
	}()
}

func (s *ServerStream[T]) queryHead(query string, limit int32) {
	lister, err := s.gcpLogsClient.ListLogEntries(s.ctx, query, gcp.OrderAscending)
	if err != nil {
		s.errCh <- err
		return
	}
	if limit == 0 {
		err = s.listToChannel(lister)
		if err != nil && !errors.Is(err, io.EOF) { // Ignore EOF for listing older logs, to proceed to tailing
			s.errCh <- err
			return
		}
	} else {
		buffer, err := s.listToBuffer(lister, limit)
		if err != nil {
			s.errCh <- err
		}
		for i := range buffer {
			s.respCh <- buffer[i]
		}
		s.errCh <- io.EOF
	}
}

func (s *ServerStream[T]) queryTail(query string, limit int32) {
	lister, err := s.gcpLogsClient.ListLogEntries(s.ctx, query, gcp.OrderDescending)
	if err != nil {
		s.errCh <- err
		return
	}
	if limit == 0 {
		err = s.listToChannel(lister)
		if err != nil {
			s.errCh <- err
			return
		}
	} else {
		buffer, err := s.listToBuffer(lister, limit)
		if err != nil {
			s.errCh <- err
		}
		// iterate over the buffer in reverse order to send the oldest resps first
		for i := len(buffer) - 1; i >= 0; i-- {
			s.respCh <- buffer[i]
		}
		s.errCh <- io.EOF
	}
}

func (s *ServerStream[T]) listToBuffer(lister gcp.Lister, limit int32) ([]*T, error) {
	received := 0
	buffer := make([]*T, 0, limit)
	for range limit {
		entry, err := lister.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return buffer, nil
			}
			return nil, err
		}
		resps, err := s.parseAndFilter(entry)
		if err != nil {
			return nil, err
		}
		buffer = append(buffer, resps...)
		received += len(resps)
	}
	return buffer, nil
}

func (s *ServerStream[T]) listToChannel(lister gcp.Lister) error {
	for {
		entry, err := lister.Next()
		if err != nil {
			return err
		}
		resps, err := s.parseAndFilter(entry)
		if err != nil {
			return err
		}
		for _, resp := range resps {
			s.respCh <- resp
		}
	}
}

func (s *ServerStream[T]) parseAndFilter(entry *loggingpb.LogEntry) ([]*T, error) {
	resps, err := s.parse(entry)
	if err != nil {
		return nil, err
	}
	newResps := make([]*T, 0, len(resps))
	for _, resp := range resps {
		include := true
		for _, f := range s.filters {
			if resp = f(resp); resp == nil {
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

func (s *ServerStream[T]) AddCustomQuery(query string) {
	s.query.AddQuery(query)
}

func (s *ServerStream[T]) GetQuery() string {
	return s.query.GetQuery()
}

func (s *ServerStream[T]) Err() error {
	return s.lastErr
}

func (s *ServerStream[T]) Msg() *T {
	return s.lastResp
}

type LogStream struct {
	*ServerStream[defangv1.TailResponse]
}

func NewLogStream(ctx context.Context, gcpLogsClient GcpLogsClient, services []string) (*LogStream, error) {
	restoreServiceName := getServiceNameRestorer(services, gcp.SafeLabelValue,
		func(entry *defangv1.TailResponse) string { return entry.Service },
		func(entry *defangv1.TailResponse, name string) *defangv1.TailResponse {
			entry.Service = name
			return entry
		})

	ss, err := NewServerStream(ctx, gcpLogsClient, getLogEntryParser(ctx, gcpLogsClient), restoreServiceName)
	if err != nil {
		return nil, err
	}

	ss.query = NewLogQuery(gcpLogsClient.GetProjectID())
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

type SubscribeStream struct {
	*ServerStream[defangv1.SubscribeResponse]
}

func getServiceNameRestorer[T any](services []string, encode func(string) string, extract func(T) string, update func(T, string) T) func(T) T {
	mapping := make(map[string]string, len(services))
	for _, service := range services {
		mapping[encode(service)] = service
	}
	return func(entry T) T {
		name := extract(entry)
		if restored, ok := mapping[name]; ok {
			name = restored
		}
		return update(entry, name)
	}
}

func NewSubscribeStream(ctx context.Context, driver GcpLogsClient, waitForCD bool, etag string, services []string, filters ...LogFilter[*defangv1.SubscribeResponse]) (*SubscribeStream, error) {
	filters = append(filters, getServiceNameRestorer(services, gcp.SafeLabelValue,
		func(entry *defangv1.SubscribeResponse) string { return entry.Name },
		func(entry *defangv1.SubscribeResponse, name string) *defangv1.SubscribeResponse {
			entry.Name = name
			return entry
		}),
	)

	ss, err := NewServerStream(ctx, driver, getActivityParser(ctx, driver, waitForCD, etag), filters...)
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
	s.query.AddCloudBuildActivityQuery()
}

var cdExecutionNamePattern = regexp.MustCompile(`^defang-cd-[a-z0-9]{5}$`)

func getLogEntryParser(ctx context.Context, gcpClient GcpLogsClient) func(entry *loggingpb.LogEntry) ([]*defangv1.TailResponse, error) {
	envCache := make(map[string]map[string]string)
	cdStarted := false
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
		if strings.Contains(strings.ToLower(msg), "error:") {
			stderr = true
		}

		var serviceName, etag, host string
		var buildTags []string
		serviceName = entry.Labels["defang-service"]
		executionName := entry.Labels["run.googleapis.com/execution_name"]
		if entry.Labels["build_tags"] != "" {
			buildTags = strings.Split(entry.Labels["build_tags"], ",")
		}
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
				env, err = gcpClient.GetExecutionEnv(ctx, executionName)
				if err != nil {
					return nil, fmt.Errorf("failed to get execution environment variables: %w", err)
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
		} else if len(buildTags) > 0 {
			var bt gcp.BuildTag
			if err := bt.Parse(buildTags); err != nil {
				return nil, err
			}
			serviceName = bt.Service
			etag = bt.Etag
			host = "cloudbuild"
			if bt.IsDefangCD {
				host = "pulumi"
			}
			// HACK: Detect cd start from cloudbuild logs to skip the cloud build image pulling logs
			// " ** " or "Defang: " could come first in the log message when cd starts
			if strings.HasPrefix(msg, " ** ") || strings.HasPrefix(msg, "Defang: ") {
				cdStarted = true
			}
			if !cdStarted {
				return nil, nil // Skip cloudbuild logs (like pulling cd image) before cd started
			}
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

func getActivityParser(ctx context.Context, gcpLogsClient GcpLogsClient, waitForCD bool, etag string) func(entry *loggingpb.LogEntry) ([]*defangv1.SubscribeResponse, error) {
	cdSuccess := false
	readyServices := make(map[string]string)

	computeEngineRootTriggers := make(map[string]string)

	getReadyServicesCompletedResps := func(status string) []*defangv1.SubscribeResponse {
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
			Status: status,
		})
		return resps
	}

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
				return getReadyServicesCompletedResps(auditLog.GetStatus().GetMessage()), nil // Ignore success cd status when we are waiting for service status
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
		case "build": // Cloudbuild events
			buildId := entry.Resource.Labels["build_id"]
			if buildId == "" {
				return nil, nil // Ignore activities without build id
			}
			bt, err := gcpLogsClient.GetBuildInfo(ctx, buildId) // TODO: Cache the build IDs?
			if err != nil {
				term.Warnf("failed to get build tag for build %v: %v", buildId, err)
				return nil, nil
			}

			if etag != "" && bt.Etag != etag {
				return nil, nil
			}

			if bt.IsDefangCD {
				if !entry.Operation.Last { // Ignore non-final cloud build event for CD
					return nil, nil
				}
				// When cloud build fails, the last log message is an error message
				if entry.Severity == logtype.LogSeverity_ERROR {
					return nil, client.ErrDeploymentFailed{Message: auditLog.GetStatus().GetMessage()}
				}

				cdSuccess = true
				return getReadyServicesCompletedResps(auditLog.GetStatus().String()), nil
			} else {
				var state defangv1.ServiceState
				status := ""
				if entry.Operation.First {
					state = defangv1.ServiceState_BUILD_ACTIVATING
				} else if entry.Operation.Last {
					if entry.Severity == logtype.LogSeverity_ERROR {
						state = defangv1.ServiceState_BUILD_FAILED
						if auditLog.GetStatus() != nil {
							status = auditLog.GetStatus().String()
						}
					} else {
						state = defangv1.ServiceState_BUILD_STOPPING
					}
				} else {
					state = defangv1.ServiceState_BUILD_RUNNING
				}
				if status == "" {
					status = state.String()
				}
				return []*defangv1.SubscribeResponse{{
					Name:   bt.Service,
					State:  state,
					Status: status,
				}}, nil
			}
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
