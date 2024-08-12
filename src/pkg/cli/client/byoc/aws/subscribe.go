package aws

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

var deploymentEtags = make(map[string]string)

type byocSubscribeServerStream struct {
	services []string
	etag     string
	ctx      context.Context

	ch          chan *defangv1.SubscribeResponse
	resp        *defangv1.SubscribeResponse
	err         error
	closed      bool
	closingLock sync.Mutex
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
	if len(evt.Overrides.ContainerOverrides) < 1 {
		s.error(fmt.Errorf("error parsing ECS task state change: no container overrides"))
		return
	}
	var service, etag, status string
	var state defangv1.ServiceState

	name, service, etag, err := GetEcsTaskStateChangeServiceEtag(evt)
	if err != nil {
		s.error(err)
		return
	}

	// Cache deployment id to etag mapping
	if evt.StartedBy != "" && etag != "" {
		deploymentEtags[evt.StartedBy] = etag
	}

	if name == "kaniko" {
		state = kanikoTaskStateMap[evt.LastStatus]
		if state == defangv1.ServiceState_NOT_SPECIFIED {
			return // Ignore unknown kaniko task states
		}
		if state == defangv1.ServiceState_BUILD_STOPPING {
			for _, container := range evt.Containers {
				if container.ExitCode != 0 {
					state = defangv1.ServiceState_BUILD_FAILED
				}
			}
		}
		status = "BUILD_" + evt.LastStatus + " " + evt.StoppedReason
	} else {
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
	etag, ok := deploymentEtags[evt.DeploymentId]
	if !ok {
		return // Ignore deployments that are not associated with an etag
	}
	if etag != "" && etag != s.etag {
		return // Ignore deployments that is not associated with the current etag
	}

	if len(resources) < 1 {
		s.error(fmt.Errorf("error parsing ECS deployment state change: no resources"))
		return
	}

	service, err := GetEcsDeploymetStateChangeService(resources)
	if err != nil {
		s.error(err)
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

	s.send(&defangv1.SubscribeResponse{
		Name:   service,
		Status: status,
		State:  state,
	})
}

func (s *byocSubscribeServerStream) Close() error {
	s.closingLock.Lock()
	defer s.closingLock.Unlock()
	s.closed = true
	close(s.ch)
	return nil
}

func (s *byocSubscribeServerStream) Receive() bool {
	select {
	case resp, ok := <-s.ch:
		if !ok || resp == nil {
			return false
		}
		s.resp = resp
		return true
	case <-s.ctx.Done():
		s.err = s.ctx.Err()
		return false
	}
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
	s.closingLock.Lock()
	defer s.closingLock.Unlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- resp:
	case <-s.ctx.Done():
	}
}

func GetEcsTaskStateChangeServiceEtag(evt ecs.ECSTaskStateChange) (name, service, etag string, err error) {
	// Container name is in the format of "service_etag"
	if len(evt.Overrides.ContainerOverrides) < 1 {
		return "", "", "", fmt.Errorf("no container overrides for task %v", evt.TaskArn)
	}
	for _, container := range evt.Overrides.ContainerOverrides {
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
				return "", "", "", fmt.Errorf("could not find service name from kaniko task %v", evt.TaskArn)
			}
		} else { // Service task name is in the form of "service_etag"
			i := strings.LastIndex(container.Name, "_")
			if i < 0 {
				return "", "", "", fmt.Errorf("invalid container name %q for task %v", container.Name, evt.TaskArn)
			}
			service = container.Name[:i]
			etag = container.Name[i+1:]
		}
		if service != "" {
			name = container.Name
			break
		}
	}
	return name, service, etag, nil
}

func GetEcsDeploymetStateChangeService(resources []string) (string, error) {
	if len(resources) < 1 {
		return "", fmt.Errorf("no resources")
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
		break
	}
	if service == "" {
		return "", fmt.Errorf("cannot find service name from resources %v", resources)
	}
	return service, nil
}
