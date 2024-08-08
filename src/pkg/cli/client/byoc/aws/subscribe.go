package aws

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type byocSubscribeServerStream struct {
	services []string
	etag     string

	ch              chan *defangv1.SubscribeResponse
	resp            *defangv1.SubscribeResponse
	err             error
	deploymentEtags map[string]string
	closed          bool
}

var kanikoLabelPrefix = `--label=io.defang.image.name=`
var kanikoLabelSuffix = `-image`

var kanikoTaskStateMap = map[string]defangv1.ServiceState{
	// "STOPPED":        defangv1.ServiceState_BUILD_STOPPING, // Ignored
	"DEPROVISIONING": defangv1.ServiceState_BUILD_STOPPING,
	"RUNNING":        defangv1.ServiceState_BUILD_RUNNING,
	"PENDING":        defangv1.ServiceState_BUILD_PENDING,
	"PROVISIONING":   defangv1.ServiceState_BUILD_PROVISIONING,
}

func (s *byocSubscribeServerStream) HandleEcsTaskStateChange(evt ecs.ECSTaskStateChange) {
	// Container name is in the format of "service_etag"
	if len(evt.Overrides.ContainerOverrides) < 1 {
		s.error(fmt.Errorf("error parsing ECS task state change: no container overrides"))
		return
	}
	var service, etag, status string
	var state defangv1.ServiceState
	for i, container := range evt.Overrides.ContainerOverrides {
		if container.Name == "log_router" {
			continue
		}
		if container.Name == "kaniko" { // Build task
			for _, arg := range container.Command {
				if strings.HasPrefix(arg, kanikoLabelPrefix) && strings.HasSuffix(arg, kanikoLabelSuffix) {
					service = strings.TrimPrefix(strings.TrimSuffix(arg, kanikoLabelSuffix), kanikoLabelPrefix)
					break
				}
			}
			if service == "" {
				s.error(fmt.Errorf("error parsing ECS task state change: unable to find service name from kaniko task"))
				return
			}
			state = kanikoTaskStateMap[evt.LastStatus]
			if state == defangv1.ServiceState_NOT_SPECIFIED {
				return // Ignore unknown kaniko task states
			}
			if state == defangv1.ServiceState_BUILD_STOPPING {
				if len(evt.Containers) < i+1 {
					s.error(fmt.Errorf("error parsing ECS task state change: unable to find service name from kaniko task"))
					return
				}
				if evt.Containers[i].ExitCode != 0 {
					state = defangv1.ServiceState_BUILD_FAILED
				}
			}
			status = "BUILD_" + evt.LastStatus + " " + evt.StoppedReason
		} else { // Service task name is in the form of "service_etag"
			i := strings.LastIndex(container.Name, "_")
			if i < 0 {
				s.error(fmt.Errorf("error parsing ECS task state change: invalid container name %q", container.Name))
				return
			}
			service = container.Name[:i]
			etag = container.Name[i+1:]

			if evt.StartedBy != "" {
				s.deploymentEtags[evt.StartedBy] = etag
			}

			if etag != s.etag {
				return
			}

			if evt.LastStatus == "DEACTIVATING" {
				state = defangv1.ServiceState_DEPLOYMENT_FAILED // Fast deployment fail
			} else {
				state = defangv1.ServiceState_DEPLOYMENT_PENDING // Treat all other task updates as deployment pending
			}

			status = "TASK_" + evt.LastStatus
			if evt.StoppedReason != "" {
				status += " " + evt.StoppedReason
			}
		}
		if service != "" {
			break
		}
	}

	if !slices.Contains(s.services, service) {
		return
	}

	if etag != "" && etag != s.etag {
		return
	}

	s.send(&defangv1.SubscribeResponse{
		Name:   service,
		Status: status,
		State:  state,
	})
}

func (s *byocSubscribeServerStream) HandleEcsDeploymentStateChange(evt ecs.ECSDeploymentStateChange, resources []string) {
	etag, ok := s.deploymentEtags[evt.DeploymentId]
	if !ok {
		return // Ignore deployments that are not associated with an etag
	}
	if etag != "" && etag != s.etag {
		return // Ignore deployments that is not associated with the current etag
	}

	if len(resources) < 1 {
		s.err = fmt.Errorf("error parsing ECS deployment state change: no resources")
		s.ch <- nil
		return
	}

	var service string
	// resources are in the format of "arn:aws:ecs:region:account:service/cluster/project_service-random"
	for _, resource := range resources {
		id := path.Base(resource) // project_service-random
		snStart := strings.Index(id, "_")
		snEnd := strings.LastIndex(id, "-")
		if snStart < 0 || snEnd < 0 || snStart >= snEnd {
			continue
		}
		service = id[snStart+1 : snEnd]
	}
	if service == "" {
		s.err = fmt.Errorf("error parsing ECS deployment state change: invalid resourcs name %v", resources)
		s.ch <- nil
		return
	}

	var state defangv1.ServiceState
	switch evt.EventName {
	case "SERVICE_DEPLOYMENT_COMPLETED":
		state = defangv1.ServiceState_DEPLOYMENT_COMPLETED
	case "SERVICE_DEPLOYMENT_FAILED":
		state = defangv1.ServiceState_DEPLOYMENT_FAILED
	default:
		state = defangv1.ServiceState_DEPLOYMENT_PENDING
	}

	status := evt.EventName
	if evt.Reason != "" {
		status += " " + evt.Reason
	}

	resp := &defangv1.SubscribeResponse{
		Name:   service,
		Status: status,
		State:  state,
	}
	s.ch <- resp
}

func (s *byocSubscribeServerStream) Close() error {
	s.closed = true
	close(s.ch)
	return nil
}

func (s *byocSubscribeServerStream) Receive() bool {
	resp, ok := <-s.ch
	if !ok || resp == nil {
		return false
	}
	s.resp = resp
	return true
}

func (s *byocSubscribeServerStream) Msg() *defangv1.SubscribeResponse {
	return s.resp
}

func (s *byocSubscribeServerStream) Err() error {
	return s.err
}

func (s *byocSubscribeServerStream) error(err error) {
	s.err = err
	s.send(nil)
}

func (s *byocSubscribeServerStream) send(resp *defangv1.SubscribeResponse) {
	if s.closed {
		return
	}
	s.ch <- resp
}
