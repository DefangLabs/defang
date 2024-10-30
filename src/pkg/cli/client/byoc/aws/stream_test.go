package aws

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/logs"
)

// write a test for bs.makeResponseHeader
func Test_makeResponseHeader(t *testing.T) {
	etag := "some-etag"
	services := []string{}
	ctx := context.Background()
	awsProvider := NewByocProvider(ctx, client.GrpcClient{}, "tenantx")
	bs := newByocServerStream(ctx, nil, etag, services, awsProvider)

	tests := []struct {
		Label           string
		GroupIdentifier string
		StreamName      string
		Message         string
		Mode            SSMode
		Service         string
		Host            string
		Etag            string
	}{
		{
			"cd logs",
			"arn:aws:logs:us-test-2:1234567890:log-group:defang-cd-LogGroup-6LSZet3tFnEy",
			"crun/main/39e1650bc1d84943a027d7540a610219",
			"some message",
			SSModeCd,
			"cd",
			"pulumi",
			etag,
		},
		{
			"kaniko firelens logs",
			"arn:aws:logs:us-test-2:1234567890:log-group:/Defang/golang-http/beta/builds",
			"kaniko/kaniko-firelens-b4b253d1a31747df8d1a93c3d6403fae",
			makeKanikoMessage(etag),
			SSModeFirelens,
			"kaniko",
			"ip-10-0-41-41",
			etag,
		},
		{
			"service logs straight to cloudwatch",
			"arn:aws:logs:us-test-2:1234567890:log-group:/Defang/golang-http/beta/logs",
			"app/app_kad5x8rcvezn/94f571ae0f1d464c995659a320f35bcc",
			"some message",
			SSModeAwslogs,
			"app",
			"94f571ae0f1d464c995659a320f35bcc",
			"kad5x8rcvezn",
		},
		{
			"ecs status updates",
			"arn:aws:logs:us-test-2:1234567890:log-group:/Defang/golang-http/beta/ecs",
			"981b99b7-f77a-35a7-95a7-1cc9da956a1d",
			"some message",
			SSModeECS,
			"ecs",
			"",
			etag,
		},
	}

	for _, tt := range tests {
		event := makeLogEvent(tt.GroupIdentifier, tt.StreamName, tt.Message)
		header := bs.makeResponseHeader(event)
		if header.Mode != tt.Mode {
			t.Fatalf("case %q, expected mode %v, got %v", tt.Label, tt.Mode, header.Mode)
		}
		if header.Service != tt.Service {
			t.Fatalf("case %q, expected service %q, got %q", tt.Label, tt.Service, header.Service)
		}
		if header.Host != tt.Host {
			t.Fatalf("case %q, expected host %q, got %q", tt.Label, tt.Host, header.Host)
		}
		if header.Etag != tt.Etag {
			t.Fatalf("case %q, expected etag %q, got %q", tt.Label, etag, header.Etag)
		}
	}
}

func Test_makeResponse(t *testing.T) {
	etag := "some-etag"
	ctx := context.Background()
	awsProvider := NewByocProvider(ctx, client.GrpcClient{}, "tenantx")

	tests := []struct {
		Label    string
		Header   TailResponseHeader
		Services []string
		Events   []ecs.LogEvent
		Entries  int
	}{
		{
			"cd logs",
			TailResponseHeader{
				Mode:    SSModeCd,
				Service: "cd",
				Host:    "pulumi",
				Etag:    etag,
			},
			[]string{},
			[]ecs.LogEvent{makeLogEvent("group", "stream", "message")},
			1,
		},
		{
			"kaniko firelens logs",
			TailResponseHeader{
				Mode:    SSModeFirelens,
				Service: "kaniko",
				Host:    "ip-10-0-41-41",
				Etag:    etag,
			},
			[]string{},
			[]ecs.LogEvent{makeLogEvent("group", "stream", makeKanikoMessage(etag))},
			1,
		},
		{
			"service logs straight to cloudwatch",
			TailResponseHeader{
				Mode:    SSModeAwslogs,
				Service: "app",
				Host:    "94f571ae0f1d464c995659a320f35bcc",
				Etag:    etag,
			},
			[]string{},
			[]ecs.LogEvent{makeLogEvent("group", "stream", "message")},
			1,
		},
		{
			"ecs status updates",
			TailResponseHeader{
				Mode:    SSModeECS,
				Service: "ecs",
				Host:    "",
				Etag:    etag,
			},
			[]string{},
			[]ecs.LogEvent{makeLogEvent("group", "stream", "message")},
			1,
		},
		{
			"skip kaniko firelens events with different etag",
			TailResponseHeader{
				Mode:    SSModeFirelens,
				Service: "kaniko",
				Host:    "ip-10-0-41-41",
				Etag:    etag,
			},
			[]string{},
			[]ecs.LogEvent{
				makeLogEvent("group", "stream", makeKanikoMessage(etag)),
				makeLogEvent("group", "stream", makeKanikoMessage("different-etag")),
			},
			1,
		},
	}

	for _, tt := range tests {
		bs := newByocServerStream(ctx, nil, etag, tt.Services, awsProvider)
		event := makeLogEvent("group", "stream", "message")
		response := bs.makeResponse(&tt.Header, []ecs.LogEvent{event})
		if response == nil {
			if tt.Entries != 0 {
				t.Fatalf("case %q, expected response with entries, but got no response", tt.Label)
			}
		} else {
			if response.Service != tt.Header.Service {
				t.Fatalf("case %q, expected service %q, got %q", tt.Label, tt.Header.Service, response.Service)
			}
			if response.Host != tt.Header.Host {
				t.Fatalf("case %q, expected host %q, got %q", tt.Label, tt.Header.Host, response.Host)
			}
			if response.Etag != tt.Header.Etag {
				t.Fatalf("case %q, expected etag %q, got %q", tt.Label, tt.Header.Etag, response.Etag)
			}
			if len(response.Entries) != tt.Entries {
				t.Fatalf("case %q, expected %d entry, got %d", tt.Label, tt.Entries, len(response.Entries))
			}
		}
	}
}

func makeKanikoMessage(etag string) string {
	msg := logs.FirelensMessage{
		Etag:          etag,
		Host:          "ip-10-0-41-41",
		ContainerName: "kaniko",
		Log:           "some message...",
	}

	encoded, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func makeLogEvent(groupIdentifier, streamName, message string) ecs.LogEvent {
	ingestionTime := int64(1)
	timestamp := int64(1)

	return ecs.LogEvent{
		LogGroupIdentifier: &groupIdentifier,
		LogStreamName:      &streamName,
		Message:            &message,
		IngestionTime:      &ingestionTime,
		Timestamp:          &timestamp,
	}
}
