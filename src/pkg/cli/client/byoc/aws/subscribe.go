package aws

import (
	"iter"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws/codebuild"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/cw"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

// parseSubscribeEvents converts CW log events from ECS and builds log groups
// into SubscribeResponse, filtering by etag and services.
func parseSubscribeEvents(events iter.Seq2[cw.LogEvent, error], etag types.ETag, services []string) iter.Seq2[*defangv1.SubscribeResponse, error] {
	return func(yield func(*defangv1.SubscribeResponse, error) bool) {
		for evt, err := range events {
			if err != nil {
				yield(nil, err)
				return
			}
			resp := parseSubscribeEvent(evt, etag, services)
			if resp != nil {
				if !yield(resp, nil) {
					return
				}
			}
		}
	}
}

func parseSubscribeEvent(evt cw.LogEvent, etag types.ETag, services []string) *defangv1.SubscribeResponse {
	if evt.LogGroupIdentifier == nil || evt.Message == nil {
		return nil
	}

	switch {
	case strings.HasSuffix(*evt.LogGroupIdentifier, "/ecs"):
		return parseECSSubscribeEvent(evt, etag, services)
	case strings.HasSuffix(*evt.LogGroupIdentifier, "/builds") &&
		evt.LogStreamName != nil &&
		codeBuildPrefixRegex.MatchString(*evt.LogStreamName):
		return parseCodebuildSubscribeEvent(evt, etag, services)
	default:
		return nil
	}
}

func parseECSSubscribeEvent(evt cw.LogEvent, etag types.ETag, services []string) *defangv1.SubscribeResponse {
	ecsEvt, err := ecs.ParseECSEvent([]byte(*evt.Message))
	if err != nil {
		term.Debugf("error parsing ECS event: %v", err)
		return nil
	}

	if e := ecsEvt.Etag(); e == "" || (etag != "" && e != etag) {
		return nil
	}
	if service := ecsEvt.Service(); len(services) > 0 && !slices.Contains(services, service) {
		return nil
	}

	return &defangv1.SubscribeResponse{
		Name:   ecsEvt.Service(),
		Status: ecsEvt.Status(),
		State:  ecsEvt.State(),
	}
}

func parseCodebuildSubscribeEvent(evt cw.LogEvent, etag types.ETag, services []string) *defangv1.SubscribeResponse {
	// Extract service/etag from log stream name: "<service>-image/<service>_<etag>/<build_id>"
	if evt.LogStreamName == nil {
		return nil
	}
	parts := strings.Split(*evt.LogStreamName, "/")
	if len(parts) != 3 {
		return nil
	}
	underscore := strings.LastIndexByte(parts[1], '_')
	if underscore < 0 {
		return nil
	}

	cbEtag := parts[1][underscore+1:]
	cbService := parts[0] // <service>-image
	cbHost := parts[2]    // build id

	if etag != "" && cbEtag != etag {
		return nil
	}

	service := strings.TrimSuffix(cbService, "-image")
	if len(services) > 0 && !slices.Contains(services, service) {
		return nil
	}

	entry := &defangv1.LogEntry{
		Message: *evt.Message,
		Service: cbService,
		Etag:    cbEtag,
		Host:    cbHost,
	}
	cbEvt := codebuild.ParseCodebuildEvent(entry)
	if cbEvt.State() == defangv1.ServiceState_NOT_SPECIFIED {
		return nil
	}

	return &defangv1.SubscribeResponse{
		Name:   service,
		Status: cbEvt.Status(),
		State:  cbEvt.State(),
	}
}
