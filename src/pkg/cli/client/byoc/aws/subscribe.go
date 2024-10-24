package aws

import (
	"context"
	"slices"
	"sync"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type byocSubscribeServerStream struct {
	services []string
	etag     types.ETag
	ctx      context.Context

	ch          chan *defangv1.SubscribeResponse
	resp        *defangv1.SubscribeResponse
	err         error
	closed      bool
	closingLock sync.Mutex
}

func (s *byocSubscribeServerStream) HandleECSEvent(evt ecs.Event) {
	if etag := evt.Etag(); etag == "" || etag != s.etag {
		return
	}
	if service := evt.Service(); len(s.services) > 0 && !slices.Contains(s.services, service) {
		return
	}
	s.send(&defangv1.SubscribeResponse{
		Name:   evt.Service(),
		Status: evt.Status(),
		State:  evt.State(),
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
