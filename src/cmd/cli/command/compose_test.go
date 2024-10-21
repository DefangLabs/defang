package command

import (
	"context"
	"errors"
	"testing"
	"time"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestGetUnreferencedManagedResources(t *testing.T) {

	t.Run("no services", func(t *testing.T) {
		project := []*defangv1.ServiceInfo{}
		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resources, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("one service all managed", func(t *testing.T) {
		project := []*defangv1.ServiceInfo{}
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service1", Postgres: &defangv1.Postgres{}}})

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("one service unmanaged", func(t *testing.T) {
		project := []*defangv1.ServiceInfo{}
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service1"}})

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resource, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("one service unmanaged, one service managed", func(t *testing.T) {
		project := []*defangv1.ServiceInfo{}
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service1"}})
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service2", Postgres: &defangv1.Postgres{}}})

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("two service two unmanaged", func(t *testing.T) {
		project := []*defangv1.ServiceInfo{}
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service1"}})
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service2"}})

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resource, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("one service two managed", func(t *testing.T) {
		project := []*defangv1.ServiceInfo{}
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service1", Postgres: &defangv1.Postgres{}, Redis: &defangv1.Redis{}}})

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%s)", len(managed), managed)
		}
	})
}

func TestStartTimer(t *testing.T) {
	t.Run("negative timeout", func(t *testing.T) {
		// define variables
		ctx := context.Background()
		waitTimeout := -1
		waitTimeoutAsTime := time.Duration(0) * time.Second // waitTimeout in seconds, uses zero instead of waitTimeout to avoid negative time
		calledCtx, done := context.WithCancel(context.Background())
		cancelTail := func(err error) {
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("Expected context.DeadlineExceeded error, got %v", err)
			}
			done()
		}

		// initialize a stopwatch of 500ms
		stopwatch := time.NewTimer(waitTimeoutAsTime + 500*time.Millisecond)
		defer stopwatch.Stop()

		// call the function
		startTimer(ctx, waitTimeout, cancelTail)

		select {
		case <-calledCtx.Done():
			// check when timeout occured
			t.Errorf("cancelTail shouldn't be called, but it was")
		case <-stopwatch.C: // stopwatch expires after 500ms
			// expected, the timeout does not occur
		}
	})

	t.Run("zero timeout", func(t *testing.T) {
		// define variables
		ctx := context.Background()
		waitTimeout := 0
		waitTimeoutAsTime := time.Duration(waitTimeout) * time.Second // waitTimeout in seconds
		calledCtx, done := context.WithCancel(context.Background())
		cancelTail := func(err error) {
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("Expected context.DeadlineExceeded error, got %v", err)
			}
			done()
		}

		// initialize a stopwatch of 500ms + waitTimeoutAsTime
		stopwatch := time.NewTimer(500*time.Millisecond + waitTimeoutAsTime)
		defer stopwatch.Stop()

		// call the function
		startTimer(ctx, waitTimeout, cancelTail)

		select {
		case <-calledCtx.Done():
			// expected, timeout occured within expected time frame
		case <-stopwatch.C: // stopwatch expires after 500ms + waitTimeoutAsTime
			t.Errorf("Wait-timeout did not occur within the expected time frame")
		}
	})

	t.Run("positive timeout", func(t *testing.T) {
		// define variables
		ctx := context.Background()
		waitTimeout := 1
		waitTimeoutAsTime := time.Duration(waitTimeout) * time.Second // waitTimeout in seconds
		calledCtx, done := context.WithCancel(context.Background())
		cancelTail := func(err error) {
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("Expected context.DeadlineExceeded error, got %v", err)
			}
			done()
		}

		// initialize a stopwatch for 500ms + waitTimeoutAsTime
		stopwatch := time.NewTimer(500*time.Millisecond + waitTimeoutAsTime)
		defer stopwatch.Stop()
		startTimer(ctx, waitTimeout, cancelTail)

		select {
		case <-calledCtx.Done():
			// expected, timeout occurs within expected time frame
		case <-stopwatch.C: // stopwatch expires after 500ms + waitTimeoutAsTime
			t.Errorf("Wait-timeout did not occur within the expected time frame")
		}
	})

	t.Run("context cancelled before timeout", func(t *testing.T) {
		// define variables
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		waitTimeout := 1
		waitTimeoutAsTime := time.Duration(waitTimeout) * time.Second
		calledCtx, done := context.WithCancel(context.Background())
		cancelTail := func(err error) {
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("Expected context.DeadlineExceeded error, got %v", err)
			}
			done()
		}

		// initialize a stopwatch of 500ms
		stopwatch := time.NewTimer(500*time.Millisecond + waitTimeoutAsTime)
		defer stopwatch.Stop()

		// call the function
		startTimer(ctx, waitTimeout, cancelTail)

		// manually cancel the context
		cancel()

		select {
		case <-calledCtx.Done():
			t.Errorf("cancelTail shouldn't be called, but it was")
		case <-stopwatch.C: // stopwatch expires after 500ms + waitTimeoutAsTime
			// expected, the timeout does not occur
		}
	})
}
