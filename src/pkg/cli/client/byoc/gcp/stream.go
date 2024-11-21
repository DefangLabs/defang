package gcp

import (
	"context"
	"fmt"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Stream struct {
	ctx context.Context
	gcp *gcp.Gcp

	lastEntry *loggingpb.LogEntry
	lastErr   error
	entryCh   chan *loggingpb.LogEntry
	errCh     chan error
	cancel    func()
}

func NewStream(ctx context.Context, gcp *gcp.Gcp) *Stream {
	streamCtx, cancel := context.WithCancel(ctx)
	return &Stream{
		ctx: streamCtx,
		gcp: gcp,

		entryCh: make(chan *loggingpb.LogEntry),
		errCh:   make(chan error),
		cancel:  cancel,
	}
}

func (s *Stream) Close() error {
	s.cancel() // TODO: investigate if we need to close the tailer
	return nil
}

func (s *Stream) Receive() bool {
	select {
	case s.lastEntry = <-s.entryCh:
		return true
	case s.lastErr = <-s.errCh:
		return false
	}
}

func (s *Stream) AddTailer(t *gcp.Tailer) {
	go func() {
		for {
			entry, err := t.Next(s.ctx)
			if err != nil {
				s.errCh <- err
				return
			}
			s.entryCh <- entry
		}
	}()
}

func (s *Stream) Err() error {
	return s.lastErr
}

type LogStream struct {
	*Stream
}

func NewLogStream(ctx context.Context, gcp *gcp.Gcp) *LogStream {
	return &LogStream{
		Stream: NewStream(ctx, gcp),
	}
}

func (s *LogStream) AddJobLog(ctx context.Context, executionName string) error {
	tailer, err := s.gcp.NewTailer(ctx)
	if err != nil {
		return err
	}
	if err := tailer.AddJobLog(ctx, executionName); err != nil {
		return err
	}
	s.Stream.AddTailer(tailer)
	return nil
}

func (s *LogStream) AddServiceLog(ctx context.Context, service, etag string) error {
	tailer, err := s.gcp.NewTailer(ctx)
	if err != nil {
		return err
	}
	if err := tailer.AddServiceLog(ctx, service, etag); err != nil {
		return err
	}
	s.Stream.AddTailer(tailer)
	return nil
}

func (s *LogStream) Msg() *defangv1.TailResponse {
	// fmt.Printf("LOG: %+v\n", s.lastEntry)
	return &defangv1.TailResponse{
		Entries: []*defangv1.LogEntry{},
		Service: "test",
		Etag:    "testing etag",
		Host:    "testing host",
	}
}

type SubscribeStream struct {
	*Stream
}

func NewSubscribeStream(ctx context.Context, gcp *gcp.Gcp) *SubscribeStream {
	return &SubscribeStream{
		Stream: NewStream(ctx, gcp),
	}
}

func (s *SubscribeStream) AddJobExecutionUpdate(ctx context.Context, executionName string) error {
	tailer, err := s.gcp.NewTailer(ctx)
	if err != nil {
		return err
	}
	if err := tailer.AddJobExecutionUpdate(ctx, executionName); err != nil {
		return err
	}
	s.Stream.AddTailer(tailer)
	return nil
}

func (s *SubscribeStream) AddServiceStatusUpdate(ctx context.Context, service, etag string) error {
	tailer, err := s.gcp.NewTailer(ctx)
	if err != nil {
		return err
	}
	if err := tailer.AddServiceStatusUpdate(ctx, service, etag); err != nil {
		return err
	}
	s.Stream.AddTailer(tailer)
	return nil
}

func (s *SubscribeStream) Msg() *defangv1.SubscribeResponse {
	fmt.Printf("EVT: %+v\n", s.lastEntry)
	return &defangv1.SubscribeResponse{
		Name:   "test",
		Status: "testing status",
		State:  defangv1.ServiceState_DEPLOYMENT_PENDING,
	}
}
