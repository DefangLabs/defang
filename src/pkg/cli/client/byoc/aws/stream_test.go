package aws

import (
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws/cw"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ptrString returns a pointer to the given string.
func ptrString(s string) *string {
	return &s
}

func ptrInt64(i int64) *int64 {
	return &i
}

func TestStreamToLogEvent(t *testing.T) {
	var testEtag = "hg2xsgvsldqk"
	var testdata = []struct {
		event    *cw.LogEvent
		wantResp *defangv1.TailResponse
	}{
		{
			// cd message
			event: &cw.LogEvent{
				IngestionTime:      ptrInt64(1761883448306),
				LogGroupIdentifier: ptrString("532501343364:defang-cd-LogGroup-8id1W5WpeWRu"),
				LogStreamName:      ptrString("crun/main/127bb61dd5f746918f578f32cc1d6d01"),
				Timestamp:          ptrInt64(1761883446012),
				Message:            ptrString(" ** Updating service \"flask\""),
			},
			wantResp: &defangv1.TailResponse{
				Service: "cd",
				Host:    "pulumi",
				Entries: []*defangv1.LogEntry{
					{
						Timestamp: timestamppb.New(time.Unix(1761883446, int64(12000000))),
						Message:   " ** Updating service \"flask\"",
						Stderr:    true,
					},
				},
			},
		},
		{
			// error message
			event: &cw.LogEvent{
				IngestionTime:      ptrInt64(1761883448306),
				LogGroupIdentifier: ptrString("532501343364:defang-cd-LogGroup-8id1W5WpeWRu"),
				LogStreamName:      ptrString("crun/main/127bb61dd5f746918f578f32cc1d6d01"),
				Timestamp:          ptrInt64(1761883446012),
				Message:            ptrString(" \u001b[1m\u001b[38;5;2m+ \u001b[0m aws:cloudwatch:LogGroup builds \u001b[1m\u001b[38;5;2mcreating (0s)\u001b[0m \u001b[38;5;1merror: \u001b[0m\u001b[0m  sdk-v2/provider2.go:520: sdk.helper_schema: creating CloudWatch Logs Log Group (/Defang/flask-railpack/beta/builds): operation error CloudWatch Logs: CreateLogGroup, https response error StatusCode: 400, RequestID: e324d4dd-4d1b-4f3e-9b83-18d5c8bd24b1, ResourceAlreadyExistsException: The specified log group already exists: provider=aws@6.57.0"),
			},
			wantResp: &defangv1.TailResponse{
				Service: "cd",
				Host:    "pulumi",
				Entries: []*defangv1.LogEntry{
					{
						Timestamp: timestamppb.New(time.Unix(1761883446, int64(12000000))),
						Message:   " \u001b[1m\u001b[38;5;2m+ \u001b[0m aws:cloudwatch:LogGroup builds \u001b[1m\u001b[38;5;2mcreating (0s)\u001b[0m \u001b[38;5;1merror: \u001b[0m\u001b[0m  sdk-v2/provider2.go:520: sdk.helper_schema: creating CloudWatch Logs Log Group (/Defang/flask-railpack/beta/builds): operation error CloudWatch Logs: CreateLogGroup, https response error StatusCode: 400, RequestID: e324d4dd-4d1b-4f3e-9b83-18d5c8bd24b1, ResourceAlreadyExistsException: The specified log group already exists: provider=aws@6.57.0",
						Stderr:    true,
					},
				},
			},
		},
		{
			// service message
			event: &cw.LogEvent{
				IngestionTime:      ptrInt64(1761883448306),
				LogGroupIdentifier: ptrString("532501343364:/Defang/django/beta/builds"),
				LogStreamName:      ptrString("django-image/django_hg2xsgvsldqk/fb1d2a8e-9553-497e-85e4-91a57f8b6ba6"),
				Timestamp:          ptrInt64(1761883446012),
				Message:            ptrString("#12 [7/7] RUN python manage.py collectstatic --noinput\n"),
			},
			wantResp: &defangv1.TailResponse{
				Service: "cd",
				Host:    "codebuild",
				Entries: []*defangv1.LogEntry{
					{
						Timestamp: timestamppb.New(time.Unix(1761883446, int64(12000000))),
						Message:   "#12 [7/7] RUN python manage.py collectstatic --noinput\n",
						Stderr:    false,
					},
				},
			},
		},
		{
			// ECS  message
			event: &cw.LogEvent{
				IngestionTime:      ptrInt64(1761883448306),
				LogGroupIdentifier: ptrString("532501343364:/Defang/django/beta/ecs"),
				LogStreamName:      ptrString("7127bdd6-6e73-3d4e-8c97-18c3071004af"),
				Timestamp:          ptrInt64(1761883446012),
				Message:            ptrString("{\"version\":\"0\",\"id\":\"f3a2b329-b75e-ba8b-9cc7-d4a488abc19f\",\"detail-type\":\"ECS Service Action\",\"source\":\"aws.ecs\",\"account\":\"532501343364\",\"time\":\"2025-10-31T05:35:16Z\",\"region\":\"us-west-2\",\"resources\":[\"arn:aws:ecs:us-west-2:532501343364:service/Defang-django-beta-cluster/django_django-db006d3\"],\"detail\":{\"eventType\":\"INFO\",\"eventName\":\"CAPACITY_PROVIDER_STEADY_STATE\",\"clusterArn\":\"arn:aws:ecs:us-west-2:532501343364:cluster/Defang-django-beta-cluster\",\"capacityProviderArns\":[\"arn:aws:ecs:us-west-2:532501343364:capacity-provider/FARGATE_SPOT\"],\"createdAt\":\"2025-10-31T05:35:16.536Z\"}}"),
			},
			wantResp: nil,
		},
		{
			// railpack message
			event: &cw.LogEvent{
				IngestionTime:      ptrInt64(1762148772888),
				LogGroupIdentifier: ptrString("532501343364:defang-cd-LogGroup-8id1W5WpeWRu"),
				LogStreamName:      ptrString("crun/main/a467a0afd56d44baab32bb5cceb10da0"),
				Timestamp:          ptrInt64(1762148771990),
				Message:            ptrString(" \u001b[1m\u001b[38;5;2m+ \u001b[0m aws:iam:RolePolicy code-build-role-codebuildPolicy \u001b[1m\u001b[38;5;2mcreating (0s)\u001b[0m"),
			},
			wantResp: &defangv1.TailResponse{
				Service: "cd",
				Host:    "pulumi",
				Entries: []*defangv1.LogEntry{
					{
						Timestamp: timestamppb.New(time.Unix(1762148771, int64(990000000))),
						Message:   " \u001b[1m\u001b[38;5;2m+ \u001b[0m aws:iam:RolePolicy code-build-role-codebuildPolicy \u001b[1m\u001b[38;5;2mcreating (0s)\u001b[0m",
						Stderr:    false,
					},
				},
			},
		},
		{
			// service message
			event: &cw.LogEvent{
				IngestionTime:      ptrInt64(1762144097682),
				LogGroupIdentifier: ptrString("532501343364:/Defang/django6/beta/logs"),
				LogStreamName:      ptrString("django/django_hg2xsgvsldqk/b89c0f0e35ad4357852f0d7cafd488eb"),
				Timestamp:          ptrInt64(1762144092418),
				Message:            ptrString("  Applying admin.0001_initial... OK"),
			},
			wantResp: &defangv1.TailResponse{
				Service: "django",
				Host:    "b89c0f0e35ad4357852f0d7cafd488eb",
				Entries: []*defangv1.LogEntry{
					{
						Timestamp: timestamppb.New(time.Unix(1762144092, int64(418000000))),
						Message:   "  Applying admin.0001_initial... OK",
						Stderr:    false,
					},
				},
			},
		},
	}

	var byocServiceStream = newByocServerStream(nil, testEtag, []string{"cd", "app", "django"}, nil)

	for _, td := range testdata {
		tailResp := byocServiceStream.parseEvents([]cw.LogEvent{*td.event})
		if (td.wantResp == nil) != (tailResp == nil) {
			t.Errorf("nil mismatch: expected %v, got %v", td.wantResp, tailResp)
			continue
		} else if (td.wantResp == nil) && (tailResp == nil) {
			// no enties expected
			continue
		}

		assert.Equal(t, td.wantResp.Service, tailResp.Service)
		assert.Equal(t, td.wantResp.Host, tailResp.Host)

		got := tailResp.Entries[0]
		want := td.wantResp.Entries[0]
		if !got.Timestamp.AsTime().Equal(want.Timestamp.AsTime()) {
			t.Errorf("Timestamp = %v; want %v", got.Timestamp, want.Timestamp)
		}
		if got.Message != want.Message {
			t.Errorf("Message = %v; want %v", got.Message, want.Message)
		}
		if got.Stderr != want.Stderr {
			t.Errorf("Stderr = %v; want %v", got.Stderr, want.Stderr)
		}
	}
}
