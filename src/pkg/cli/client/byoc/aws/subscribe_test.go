package aws

import (
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type mockECSEvent struct {
	etag    string
	service string
	state   defangv1.ServiceState
}

func (e *mockECSEvent) Service() string              { return e.service }
func (e *mockECSEvent) Etag() string                 { return e.etag }
func (e *mockECSEvent) Host() string                 { return "" }
func (e *mockECSEvent) Status() string               { return "" }
func (e *mockECSEvent) State() defangv1.ServiceState { return e.state }

func TestByocSubscribeServerStream(t *testing.T) {
	t.Run("ignore event with different etag", func(t *testing.T) {
		ss := &byocSubscribeServerStream{
			services: []string{"service1", "service2"},
			etag:     "etag1",
			ch:       make(chan *defangv1.SubscribeResponse),
			done:     make(chan struct{}),
		}

		go func() {
			ss.HandleECSEvent(&mockECSEvent{etag: "different-etag", service: "service1"})
			ss.Close()
		}()
		if ss.Receive() {
			t.Errorf("expected no message, but got one: %v", ss.Msg())
		}
	})

	t.Run("ignore event from a different service", func(t *testing.T) {
		ss := &byocSubscribeServerStream{
			services: []string{"service1", "service2"},
			etag:     "etag1",
			ch:       make(chan *defangv1.SubscribeResponse),
			done:     make(chan struct{}),
		}

		go func() {
			ss.HandleECSEvent(&mockECSEvent{etag: "etag1", service: "service3"})
			ss.Close()
		}()
		if ss.Receive() {
			t.Errorf("expected no message, but got one: %v", ss.Msg())
		}
	})

	t.Run("receive event from correct service and etag", func(t *testing.T) {
		ss := &byocSubscribeServerStream{
			services: []string{"service1", "service2"},
			etag:     "etag1",
			ch:       make(chan *defangv1.SubscribeResponse),
			done:     make(chan struct{}),
		}

		go func() {
			ss.HandleECSEvent(&mockECSEvent{etag: "etag1", service: "service2", state: defangv1.ServiceState_DEPLOYMENT_COMPLETED})
			ss.Close()
		}()
		if !ss.Receive() {
			t.Errorf("expected a message, but got none")
		}
		if ss.Msg().Name != "service2" {
			t.Errorf("expected service2, but got: %s", ss.Msg().Name)
		}
		if ss.Msg().State != defangv1.ServiceState_DEPLOYMENT_COMPLETED {
			t.Errorf("expected state RUNNING, but got: %s", ss.Msg().State)
		}
	})

	t.Run("multiple events", func(t *testing.T) {
		ss := &byocSubscribeServerStream{
			services: []string{"service1", "service2"},
			etag:     "etag1",
			ch:       make(chan *defangv1.SubscribeResponse),
			done:     make(chan struct{}),
		}

		go func() {
			ss.HandleECSEvent(&mockECSEvent{etag: "etag1", service: "service2", state: defangv1.ServiceState_DEPLOYMENT_PENDING})
			ss.HandleECSEvent(&mockECSEvent{etag: "etag1", service: "service1", state: defangv1.ServiceState_BUILD_ACTIVATING})
			ss.HandleECSEvent(&mockECSEvent{etag: "etag1", service: "service2", state: defangv1.ServiceState_DEPLOYMENT_COMPLETED})
			ss.Close()
		}()
		count := 0
		for ss.Receive() {
			msg := ss.Msg()
			if count == 0 && (msg.Name != "service2" || msg.State != defangv1.ServiceState_DEPLOYMENT_PENDING) {
				t.Errorf("first message mismatch, got: %v", msg)
			}
			if count == 1 && (msg.Name != "service1" || msg.State != defangv1.ServiceState_BUILD_ACTIVATING) {
				t.Errorf("second message mismatch, got: %v", msg)
			}
			if count == 2 && (msg.Name != "service2" || msg.State != defangv1.ServiceState_DEPLOYMENT_COMPLETED) {
				t.Errorf("third message mismatch, got: %v", msg)
			}
			count++
		}
		if count != 3 {
			t.Errorf("expected 3 messages, but got %d", count)
		}
	})

	t.Run("event after close", func(t *testing.T) {
		ss := &byocSubscribeServerStream{
			services: []string{"service1"},
			etag:     "etag1",
			ch:       make(chan *defangv1.SubscribeResponse),
			done:     make(chan struct{}),
		}

		ss.Close()
		ss.HandleECSEvent(&mockECSEvent{etag: "etag1", service: "service1", state: defangv1.ServiceState_DEPLOYMENT_COMPLETED})
		if ss.Receive() {
			t.Errorf("expected no message after close, but got one: %v", ss.Msg())
		}
	})
}
