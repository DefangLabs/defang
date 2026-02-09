package aws

import (
	"testing"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws/cw"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
)

func makeCWLogEvent(logGroup, streamName, message string) cw.LogEvent {
	var ts int64
	return cw.LogEvent{
		LogGroupIdentifier: awssdk.String(logGroup),
		LogStreamName:      awssdk.String(streamName),
		Message:            awssdk.String(message),
		Timestamp:          &ts,
	}
}

// ECS Task State Change with etag in container override name: service1_etag1
const ecsTaskStateChange = `{"version":"0","id":"abc","detail-type":"ECS Task State Change","source":"aws.ecs","account":"123","time":"2024-01-01T00:00:00Z","region":"us-west-2","resources":["arn:aws:ecs:us-west-2:123:task/cluster/taskid"],"detail":{"lastStatus":"DEACTIVATING","stoppedReason":"","taskArn":"arn:aws:ecs:us-west-2:123:task/cluster/taskid","containers":[],"overrides":{"containerOverrides":[{"name":"service1_etag1","command":[]}]},"startedBy":"ecs-svc/deploy1"}}`

func TestParseSubscribeEvent_ECS(t *testing.T) {
	t.Run("matches etag and service", func(t *testing.T) {
		evt := makeCWLogEvent("arn:aws:logs:us-west-2:123:log-group:/ecs", "stream-id", ecsTaskStateChange)
		resp := parseSubscribeEvent(evt, "etag1", []string{"service1"})
		if resp == nil {
			t.Fatal("expected a response")
		}
		if resp.Name != "service1" {
			t.Errorf("expected service1, got %s", resp.Name)
		}
		if resp.State != defangv1.ServiceState_DEPLOYMENT_FAILED {
			t.Errorf("expected DEPLOYMENT_FAILED, got %s", resp.State)
		}
	})

	t.Run("no etag filter passes matching events", func(t *testing.T) {
		evt := makeCWLogEvent("arn:aws:logs:us-west-2:123:log-group:/ecs", "stream-id", ecsTaskStateChange)
		resp := parseSubscribeEvent(evt, "", nil)
		if resp == nil {
			t.Fatal("expected a response")
		}
		if resp.Name != "service1" {
			t.Errorf("expected service1, got %s", resp.Name)
		}
	})

	t.Run("filters by etag", func(t *testing.T) {
		evt := makeCWLogEvent("arn:aws:logs:us-west-2:123:log-group:/ecs", "stream-id", ecsTaskStateChange)
		resp := parseSubscribeEvent(evt, "different-etag", nil)
		if resp != nil {
			t.Errorf("expected nil for different etag, got %v", resp)
		}
	})

	t.Run("filters by service", func(t *testing.T) {
		evt := makeCWLogEvent("arn:aws:logs:us-west-2:123:log-group:/ecs", "stream-id", ecsTaskStateChange)
		resp := parseSubscribeEvent(evt, "etag1", []string{"other-service"})
		if resp != nil {
			t.Errorf("expected nil for different service, got %v", resp)
		}
	})

	t.Run("non-ecs log group ignored", func(t *testing.T) {
		evt := makeCWLogEvent("arn:aws:logs:us-west-2:123:log-group:/logs", "stream-id", ecsTaskStateChange)
		resp := parseSubscribeEvent(evt, "etag1", nil)
		if resp != nil {
			t.Errorf("expected nil for non-ecs log group, got %v", resp)
		}
	})
}

func TestParseSubscribeEvent_Codebuild(t *testing.T) {
	t.Run("build activating", func(t *testing.T) {
		evt := makeCWLogEvent(
			"arn:aws:logs:us-west-2:123:log-group:tenant/proj/builds",
			"worker-image/worker_etag123/build-id-1",
			"Running on CodeBuild",
		)
		resp := parseSubscribeEvent(evt, "", nil)
		if resp == nil {
			t.Fatal("expected a response")
		}
		if resp.Name != "worker" {
			t.Errorf("expected worker, got %s", resp.Name)
		}
		if resp.State != defangv1.ServiceState_BUILD_ACTIVATING {
			t.Errorf("expected BUILD_ACTIVATING, got %s", resp.State)
		}
	})

	t.Run("build failed", func(t *testing.T) {
		evt := makeCWLogEvent(
			"arn:aws:logs:us-west-2:123:log-group:tenant/proj/builds",
			"api-image/api_etag456/build-id-2",
			"Phase complete: BUILD State: FAILED",
		)
		resp := parseSubscribeEvent(evt, "", nil)
		if resp == nil {
			t.Fatal("expected a response")
		}
		if resp.Name != "api" {
			t.Errorf("expected api, got %s", resp.Name)
		}
		if resp.State != defangv1.ServiceState_BUILD_FAILED {
			t.Errorf("expected BUILD_FAILED, got %s", resp.State)
		}
	})

	t.Run("filters by etag", func(t *testing.T) {
		evt := makeCWLogEvent(
			"arn:aws:logs:us-west-2:123:log-group:tenant/proj/builds",
			"worker-image/worker_etag123/build-id-1",
			"Running on CodeBuild",
		)
		resp := parseSubscribeEvent(evt, "different-etag", nil)
		if resp != nil {
			t.Errorf("expected nil for different etag, got %v", resp)
		}
	})

	t.Run("filters by service", func(t *testing.T) {
		evt := makeCWLogEvent(
			"arn:aws:logs:us-west-2:123:log-group:tenant/proj/builds",
			"worker-image/worker_etag123/build-id-1",
			"Running on CodeBuild",
		)
		resp := parseSubscribeEvent(evt, "", []string{"api"})
		if resp != nil {
			t.Errorf("expected nil for different service, got %v", resp)
		}
	})

	t.Run("unrecognized message ignored", func(t *testing.T) {
		evt := makeCWLogEvent(
			"arn:aws:logs:us-west-2:123:log-group:tenant/proj/builds",
			"worker-image/worker_etag123/build-id-1",
			"Some random log line",
		)
		resp := parseSubscribeEvent(evt, "", nil)
		if resp != nil {
			t.Errorf("expected nil for NOT_SPECIFIED state, got %v", resp)
		}
	})
}

func TestParseSubscribeEvents(t *testing.T) {
	events := []cw.LogEvent{
		makeCWLogEvent(
			"arn:aws:logs:us-west-2:123:log-group:tenant/proj/builds",
			"worker-image/worker_etag1/build-id",
			"Running on CodeBuild",
		),
		makeCWLogEvent(
			"arn:aws:logs:us-west-2:123:log-group:tenant/proj/builds",
			"worker-image/worker_etag1/build-id",
			"Some random log line", // NOT_SPECIFIED state, should be filtered
		),
		makeCWLogEvent(
			"arn:aws:logs:us-west-2:123:log-group:/ecs",
			"stream-id",
			ecsTaskStateChange,
		),
	}

	iter := func(yield func(cw.LogEvent, error) bool) {
		for _, evt := range events {
			if !yield(evt, nil) {
				return
			}
		}
	}

	var results []*defangv1.SubscribeResponse
	for resp, err := range parseSubscribeEvents(iter, "", nil) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		results = append(results, resp)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "worker" || results[0].State != defangv1.ServiceState_BUILD_ACTIVATING {
		t.Errorf("first result mismatch: %v", results[0])
	}
	if results[1].Name != "service1" || results[1].State != defangv1.ServiceState_DEPLOYMENT_FAILED {
		t.Errorf("second result mismatch: %v", results[1])
	}
}
