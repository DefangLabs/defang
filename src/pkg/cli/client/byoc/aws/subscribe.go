package aws

import (
	"slices"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type byocSubscribeServerStream struct {
	services []string
	etag     types.ETag

	ch   chan *defangv1.SubscribeResponse
	resp *defangv1.SubscribeResponse
	err  error
	done chan struct{}
}

func (s *byocSubscribeServerStream) HandleCodebuildEvent(evt ecs.Event) {
	resp := defangv1.SubscribeResponse{
		Name:   evt.Service(),
		Status: evt.Status(),
		State:  evt.State(),
	}
	select {
	case s.ch <- &resp:
	case <-s.done:
	}
}

func (s *byocSubscribeServerStream) HandleECSEvent(evt ecs.Event) {
	if etag := evt.Etag(); etag == "" || etag != s.etag {
		return
	}
	if service := evt.Service(); len(s.services) > 0 && !slices.Contains(s.services, service) {
		return
	}
	resp := defangv1.SubscribeResponse{
		Name:   evt.Service(),
		Status: evt.Status(),
		State:  evt.State(),
	}
	select {
	case s.ch <- &resp:
	case <-s.done:
	}
}

func (s *byocSubscribeServerStream) Close() error {
	close(s.done)
	return nil
}

func (s *byocSubscribeServerStream) Receive() bool {
	select {
	case resp := <-s.ch:
		s.resp = resp
		return true
	case <-s.done:
		return false
	}
}

func (s *byocSubscribeServerStream) Msg() *defangv1.SubscribeResponse {
	return s.resp
}

func (s *byocSubscribeServerStream) Err() error {
	return s.err
}
