package cw

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/smithy-go/ptr"
	"github.com/stretchr/testify/assert"
)

func TestLogGroupIdentifier(t *testing.T) {
	arn := "arn:aws:logs:us-west-2:123456789012:log-group:/LOG/GROUP/NAME:*"
	expected := "arn:aws:logs:us-west-2:123456789012:log-group:/LOG/GROUP/NAME"
	if got := getLogGroupIdentifier(arn); got != expected {
		t.Errorf("Expected %q, but got %q", expected, got)
	}
	if got := getLogGroupIdentifier(expected); got != expected {
		t.Errorf("Expected %q, but got %q", expected, got)
	}
}

type mockFiltererTailer struct {
	filteredLogEvents []types.FilteredLogEvent
}

func (m *mockFiltererTailer) FilterLogEvents(ctx context.Context, input *cloudwatchlogs.FilterLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	return &cloudwatchlogs.FilterLogEventsOutput{
		Events: m.filteredLogEvents,
	}, nil
}

func (m *mockFiltererTailer) StartLiveTail(ctx context.Context, input *cloudwatchlogs.StartLiveTailInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.StartLiveTailOutput, error) {
	// I cannot figure out how to mock a StartLiveTailOutput with a working
	// EventStream, so I am just returning a ResourceNotFoundException for
	// testing purposes. This kind of sucks because we have code to handle
	// this error. We continue to poll for the log group until it exists.
	// That means that this test only tests that we can poll for the log group.
	return nil, &types.ResourceNotFoundException{
		Message: ptr.String("The specified log group does not exist."),
	}
}

// This is a pretty bad test. It ends up only testing that we can poll for the log group.
// because our mock StartLiveTail always returns ResourceNotFoundException.
// That means the test repeatedly tries to open a stream until we call Close() on it.
func TestQueryAndTailLogGroups(t *testing.T) {
	logGroups := []LogGroupInput{
		{
			LogGroupARN: "arn:aws:logs:us-test-2:123456789012:log-group:/defang/test/loggroup1:*",
		},
	}
	mockFiltererTailer := &mockFiltererTailer{}
	e, err := QueryAndTailLogGroups(t.Context(), mockFiltererTailer, time.Now(), time.Time{}, logGroups...)
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}
	if e.Err() != nil {
		t.Errorf("Expected no error, but got: %v", e.Err())
	}
	err = e.Close()
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}
	_, ok := <-e.Events()
	if ok {
		t.Error("Expected channel to be closed")
	}
}

func makeMockLogEvents(n int) []types.FilteredLogEvent {
	events := make([]types.FilteredLogEvent, n)
	for i := range events {
		events[i] = types.FilteredLogEvent{
			Message:   ptr.String("Log event " + strconv.Itoa(i+1)),
			Timestamp: ptr.Int64(time.Now().UnixMilli()),
		}
	}
	return events
}

func TestQueryLogGroups(t *testing.T) {
	tests := []struct {
		limit            int
		since            time.Time
		until            time.Time
		expectedMessages []string
	}{
		{
			limit:            2,
			since:            time.Time{},
			until:            time.Time{},
			expectedMessages: []string{"Log event 2", "Log event 3"},
		},
		{
			limit:            2,
			since:            time.Now(),
			until:            time.Time{},
			expectedMessages: []string{"Log event 1", "Log event 2"},
		},
		{
			limit:            2,
			since:            time.Time{},
			until:            time.Now(),
			expectedMessages: []string{"Log event 2", "Log event 3"},
		},
	}

	for _, tt := range tests {
		logEvents := makeMockLogEvents(tt.limit + 1)
		logGroups := []LogGroupInput{
			{
				LogGroupARN: "arn:aws:logs:us-test-2:123456789012:log-group:/defang/test/loggroup1:*",
			},
		}
		mockFiltererTailer := &mockFiltererTailer{
			filteredLogEvents: logEvents,
		}
		eventsCh, errsCh := QueryLogGroups(
			t.Context(),
			mockFiltererTailer,
			tt.since,
			tt.until,
			// #nosec G115 limit is small
			int32(tt.limit),
			logGroups...,
		)

		collectedMessages := make([]string, 0)
		for {
			event, ok := <-eventsCh
			if !ok {
				break
			}
			collectedMessages = append(collectedMessages, *event.Message)
		}
		assert.Len(t, collectedMessages, tt.limit)
		for i, expectedMsg := range tt.expectedMessages {
			assert.Equal(t, expectedMsg, collectedMessages[i])
		}

		select {
		case err, ok := <-errsCh:
			if ok && err != nil {
				t.Errorf("Expected no error, but got: %v", err)
			}
		default:
			// No error received, as expected
		}
	}
}
