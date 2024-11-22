package gcp

import (
	"testing"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestParseAcitivityEntry(t *testing.T) {
	serviceUpdateEntry := &loggingpb.LogEntry{
		Resource: &monitoredres.MonitoredResource{
			Type: "cloud_run_revision",
		},
		Payload: &loggingpb.LogEntry_ProtoPayload{
			ProtoPayload: &anypb.Any{
				TypeUrl: "type.googleapis.com/google.cloud.audit.AuditLog",
				Value:   []byte("\x12C\x12AReady condition status changed to True for Service nginx-d5a1fde.:\x12run.googleapis.comB\x18/Services.ReplaceServiceZ8namespaces/defang-cd-idhk6xblr21o/services/nginx-d5a1fde\x8a\x01\xbd\x13\n\xf4\x06\n\x08metadata\x12\xe7\x06*\xe4\x06\n\x17\n\x04name\x12\x0f\x1a\rnginx-d5a1fde\n\x1b\n\tnamespace\x12\x0e\x1a\x0c311770422787\nY\n\x08selfLink\x12M\x1aK/apis/serving.knative.dev/v1/namespaces/311770422787/services/nginx-d5a1fde\n-\n\x03uid\x12&\x1a$7898d6aa-c166-4e3b-9c3d-bc17679d4062\n \n\x0fresourceVersion\x12\r\x1a\x0bAAYnZ8RDUoE\n\x17\n\ngeneration\x12\t\x11\x00\x00\x00\x00\x00\x00\x14@\n2\n\x11creationTimestamp\x12\x1d\x1a\x1b2024-11-21T07:45:28.592453Z\n<\n\x06labels\x122*0\n.\n\x1dcloud.googleapis.com/location\x12\r\x1a\x0bus-central1\n\xf4\x03\n\x0bannotations\x12\xe4\x03*\xe1\x03\nY\n\x1bserving.knative.dev/creator\x12:\x1a8defang-cd@defang-cd-idhk6xblr21o.iam.gserviceaccount.com\n^\n serving.knative.dev/lastModifier\x12:\x1a8defang-cd@defang-cd-idhk6xblr21o.iam.gserviceaccount.com\nI\n\x1frun.googleapis.com/operation-id\x12&\x1a$b8a4b22e-554a-42e1-ad3d-b667b4318fef\n#\n\x1arun.googleapis.com/ingress\x12\x05\x1a\x03all\n*\n!run.googleapis.com/ingress-status\x12\x05\x1a\x03all\n\x87\x01\n\x17run.googleapis.com/urls\x12l\x1aj[\"https://nginx-d5a1fde-311770422787.us-central1.run.app\",\"https://nginx-d5a1fde-7zmbqrb66a-uc.a.run.app\"]\n&\n\napiVersion\x12\x18\x1a\x16serving.knative.dev/v1\n\x11\n\x04kind\x12\t\x1a\x07Service\n\x86\x06\n\x04spec\x12\xfd\x05*\xfa\x05\n\xb8\x05\n\x08template\x12\xab\x05*\xa8\x05\n\xad\x01\n\x08metadata\x12\xa0\x01*\x9d\x01\n]\n\x06labels\x12S*Q\n\x1d\n\x0bdefang-etag\x12\x0e\x1a\x0c15srr7n4f3ep\n0\n#run.googleapis.com/startupProbeType\x12\t\x1a\x07Default\n<\n\x0bannotations\x12-*+\n)\n autoscaling.knative.dev/maxScale\x12\x05\x1a\x03100\n\xf5\x03\n\x04spec\x12\xec\x03*\xe9\x03\n!\n\x14containerConcurrency\x12\t\x11\x00\x00\x00\x00\x00\x00T@\n\x1b\n\x0etimeoutSeconds\x12\t\x11\x00\x00\x00\x00\x00\xc0r@\n`\n\x12serviceAccountName\x12J\x1aHnginxproj-service-account@defang-cd-idhk6xblr21o.iam.gserviceaccount.com\n\xc4\x02\n\ncontainers\x12\xb5\x022\xb2\x02\n\xaf\x02*\xac\x02\n\x19\n\x05image\x12\x10\x1a\x0enginx:1-alpine\n<\n\x05ports\x12321\n/*-\n\x0f\n\x04name\x12\x07\x1a\x05http1\n\x1a\n\rcontainerPort\x12\t\x11\x00\x00\x00\x00\x00\x00T@\n@\n\tresources\x123*1\n/\n\x06limits\x12%*#\n\x0e\n\x03cpu\x12\x07\x1a\x051000m\n\x11\n\x06memory\x12\x07\x1a\x05512Mi\n\x8e\x01\n\x0cstartupProbe\x12~*|\n\x1b\n\x0etimeoutSeconds\x12\t\x11\x00\x00\x00\x00\x00\x00n@\n\x1a\n\rperiodSeconds\x12\t\x11\x00\x00\x00\x00\x00\x00n@\n\x1d\n\x10failureThreshold\x12\t\x11\x00\x00\x00\x00\x00\x00\xf0?\n\"\n\ttcpSocket\x12\x15*\x13\n\x11\n\x04port\x12\t\x11\x00\x00\x00\x00\x00\x00T@\n=\n\x07traffic\x12220\n.*,\n\x14\n\x07percent\x12\t\x11\x00\x00\x00\x00\x00\x00Y@\n\x14\n\x0elatestRevision\x12\x02 \x01\n\xc3\x05\n\x06status\x12\xb8\x05*\xb5\x05\n\x1f\n\x12observedGeneration\x12\t\x11\x00\x00\x00\x00\x00\x00\x14@\n\xba\x02\n\nconditions\x12\xab\x022\xa8\x02\nZ*X\n\x0f\n\x04type\x12\x07\x1a\x05Ready\n\x10\n\x06status\x12\x06\x1a\x04True\n3\n\x12lastTransitionTime\x12\x1d\x1a\x1b2024-11-21T08:08:08.125057Z\nh*f\n\x1d\n\x04type\x12\x15\x1a\x13ConfigurationsReady\n\x10\n\x06status\x12\x06\x1a\x04True\n3\n\x12lastTransitionTime\x12\x1d\x1a\x1b2024-11-21T08:08:06.853770Z\n`*^\n\x15\n\x04type\x12\r\x1a\x0bRoutesReady\n\x10\n\x06status\x12\x06\x1a\x04True\n3\n\x12lastTransitionTime\x12\x1d\x1a\x1b2024-11-21T08:08:08.092077Z\n4\n\x17latestReadyRevisionName\x12\x19\x1a\x17nginx-d5a1fde-00005-xwd\n6\n\x19latestCreatedRevisionName\x12\x19\x1a\x17nginx-d5a1fde-00005-xwd\nh\n\x07traffic\x12]2[\nY*W\n)\n\x0crevisionName\x12\x19\x1a\x17nginx-d5a1fde-00005-xwd\n\x14\n\x07percent\x12\t\x11\x00\x00\x00\x00\x00\x00Y@\n\x14\n\x0elatestRevision\x12\x02 \x01\n6\n\x03url\x12/\x1a-https://nginx-d5a1fde-7zmbqrb66a-uc.a.run.app\nE\n\x07address\x12:*8\n6\n\x03url\x12/\x1a-https://nginx-d5a1fde-7zmbqrb66a-uc.a.run.app\n:\n\x05@type\x121\x1a/type.googleapis.com/google.cloud.run.v1.Service")},
		},
	}

	resp, err := ParseActivityEntry(serviceUpdateEntry)
	if err != nil {
		t.Fatalf("ParseActivityEntry() failed: %v", err)
	}
	if resp == nil {
		t.Fatal("ParseActivityEntry() returned nil response")
	}
	if resp.Name != "nginx" {
		t.Errorf("ParseActivityEntry() returned unexpected name: %s, wanted nginx", resp.Name)
	}
	if resp.State != defangv1.ServiceState_DEPLOYMENT_COMPLETED {
		t.Errorf("ParseActivityEntry() returned unexpected state: %v, wanted DEPLOYMENT_COMPLETED", resp.State)
	}
	if resp.Status != "Ready condition status changed to True for Service nginx-d5a1fde." {
		t.Errorf("ParseActivityEntry() returned unexpected status: %s", resp.Status)
	}

	jobRunEntry := &loggingpb.LogEntry{
		Resource: &monitoredres.MonitoredResource{
			Type: "cloud_run_job",
		},
		Payload: &loggingpb.LogEntry_ProtoPayload{
			ProtoPayload: &anypb.Any{
				TypeUrl: "type.googleapis.com/google.cloud.audit.AuditLog",
				Value:   []byte("\x127\x125Execution defang-cd-fqsn2 has completed successfully.:\x12run.googleapis.comB\x0c/Jobs.RunJobZ<namespaces/defang-cd-idhk6xblr21o/executions/defang-cd-fqsn2\x8a\x01\xe6\x1d\n\xa0\x08\n\x08metadata\x12\x93\x08*\x90\x08\n\x19\n\x04name\x12\x11\x1a\x0fdefang-cd-fqsn2\n\x1b\n\tnamespace\x12\x0e\x1a\x0c311770422787\n\\\n\x08selfLink\x12P\x1aN/apis/run.googleapis.com/v1/namespaces/311770422787/executions/defang-cd-fqsn2\n-\n\x03uid\x12&\x1a$1170fc0f-dd1c-4f1f-bb90-b493aa319e76\n \n\x0fresourceVersion\x12\r\x1a\x0bAAYnZ8S7wzw\n\x17\n\ngeneration\x12\t\x11\x00\x00\x00\x00\x00\x00\xf0?\n2\n\x11creationTimestamp\x12\x1d\x1a\x1b2024-11-21T08:07:33.827856Z\n\x90\x02\n\x06labels\x12\x85\x02*\x82\x02\n%\n\x16run.googleapis.com/job\x12\x0b\x1a\tdefang-cd\n;\n%run.googleapis.com/jobResourceVersion\x12\x12\x1a\x101732176011875641\n'\n run.googleapis.com/jobGeneration\x12\x03\x1a\x016\nC\n\x19run.googleapis.com/jobUid\x12&\x1a$d607d42a-60c3-4e76-ac35-6ffc273b4237\n.\n\x1dcloud.googleapis.com/location\x12\r\x1a\x0bus-central1\n\xff\x01\n\x0bannotations\x12\xef\x01*\xec\x01\n2\n(run.googleapis.com/execution-environment\x12\x06\x1a\x04gen2\n2\n\x1arun.googleapis.com/creator\x12\x14\x1a\x12edwardrf@gmail.com\n7\n\x1frun.googleapis.com/lastModifier\x12\x14\x1a\x12edwardrf@gmail.com\nI\n\x1frun.googleapis.com/operation-id\x12&\x1a$a78376a5-0579-48ac-8a50-c5d68af704d0\n\xc4\x01\n\x0fownerReferences\x12\xb0\x012\xad\x01\n\xaa\x01*\xa7\x01\n\r\n\x04kind\x12\x05\x1a\x03Job\n\x13\n\x04name\x12\x0b\x1a\tdefang-cd\n-\n\x03uid\x12&\x1a$d607d42a-60c3-4e76-ac35-6ffc273b4237\n&\n\napiVersion\x12\x18\x1a\x16serving.knative.dev/v1\n\x10\n\ncontroller\x12\x02 \x01\n\x18\n\x12blockOwnerDeletion\x12\x02 \x01\n%\n\napiVersion\x12\x17\x1a\x15run.googleapis.com/v1\n\x13\n\x04kind\x12\x0b\x1a\tExecution\n\xe2\x0b\n\x04spec\x12\xd9\x0b*\xd6\x0b\n\x18\n\x0bparallelism\x12\t\x11\x00\x00\x00\x00\x00\x00\xf0?\n\x16\n\ttaskCount\x12\t\x11\x00\x00\x00\x00\x00\x00\xf0?\n\xa1\x0b\n\x08template\x12\x94\x0b*\x91\x0b\n\x8e\x0b\n\x04spec\x12\x85\x0b*\x82\x0b\n\xfa\t\n\ncontainers\x12\xeb\t2\xe8\t\n\xe5\t*\xe2\t\n\x0e\n\x04name\x12\x06\x1a\x04main\n\x96\x01\n\x05image\x12\x8c\x01\x1a\x89\x01us-central1-docker.pkg.dev/defang-cd-idhk6xblr21o/defang-cd/gcpcd@sha256:7695e14f2e26656bf9ef1370457ebb50282b5a6b1b5cda7ede4266918e35dd22\n\xd4\x03\n\x04args\x12\xcb\x032\xc8\x03\n\x04\x1a\x02up\n\xbf\x03\x1a\xbc\x03CjkKDQoFbmdpbngyBAhQGAEaCW5naW54cHJvaiIMMTVzcnI3bjRmM2VwKg1VUERBVEVfUVVFVUVEeAcijwJuYW1lOiBuZ2lueHByb2oKc2VydmljZXM6CiAgbmdpbng6CiAgICBpbWFnZTogbmdpbng6MS1hbHBpbmUKICAgIG5ldHdvcmtzOgogICAgICBkZWZhdWx0OiBudWxsCiAgICBwb3J0czoKICAgICAgLSBtb2RlOiBpbmdyZXNzCiAgICAgICAgdGFyZ2V0OiA4MAogICAgICAgIHB1Ymxpc2hlZDogIjgwODAiCiAgICAgICAgcHJvdG9jb2w6IHRjcAogICAgICAgIGFwcF9wcm90b2NvbDogaHR0cApuZXR3b3JrczoKICBkZWZhdWx0OgogICAgbmFtZTogbmdpbnhwcm9qX2RlZmF1bHQK\n\x14\n\nworkingDir\x12\x06\x1a\x04/app\n\x8a\x04\n\x03env\x12\x82\x042\xff\x03\n.*,\n\x17\n\x04name\x12\x0f\x1a\rDEFANG_PREFIX\n\x11\n\x05value\x12\x08\x1a\x06defang\n7*5\n\"\n\x04name\x12\x1a\x1a\x18PULUMI_CONFIG_PASSPHRASE\n\x0f\n\x05value\x12\x06\x1a\x04asdf\n<*:\n\x15\n\x04name\x12\r\x1a\x0bGCP_PROJECT\n!\n\x05value\x12\x18\x1a\x16defang-cd-idhk6xblr21o\n+*)\n\x14\n\x04name\x12\x0c\x1a\nDEFANG_ORG\n\x11\n\x05value\x12\x08\x1a\x06defang\n$*\"\n\x0f\n\x04name\x12\x07\x1a\x05STACK\n\x0f\n\x05value\x12\x06\x1a\x04beta\nH*F\n\x1c\n\x04name\x12\x14\x1a\x12PULUMI_BACKEND_URL\n&\n\x05value\x12\x1d\x1a\x1bgs://defang-cd-hrl8aoofvmeq\n,**\n\x10\n\x04name\x12\x08\x1a\x06REGION\n\x16\n\x05value\x12\r\x1a\x0bus-central1\n5*3\n\x10\n\x04name\x12\x08\x1a\x06DOMAIN\n\x1f\n\x05value\x12\x16\x1a\x14nginxproj.defang.dev\n'*%\n\x12\n\x04name\x12\n\x1a\x08NO_COLOR\n\x0f\n\x05value\x12\x06\x1a\x04true\n+*)\n\x11\n\x04name\x12\t\x1a\x07PROJECT\n\x14\n\x05value\x12\x0b\x1a\tnginxproj\n=\n\tresources\x120*.\n,\n\x06limits\x12\"* \n\n\n\x03cpu\x12\x03\x1a\x012\n\x12\n\x06memory\x12\x08\x1a\x061953Mi\n\x17\n\nmaxRetries\x12\t\x11\x00\x00\x00\x00\x00\x00\x00\x00\n\x18\n\x0etimeoutSeconds\x12\x06\x1a\x041800\nP\n\x12serviceAccountName\x12:\x1a8defang-cd@defang-cd-idhk6xblr21o.iam.gserviceaccount.com\n\xe1\x08\n\x06status\x12\xd6\x08*\xd3\x08\n\x1f\n\x12observedGeneration\x12\t\x11\x00\x00\x00\x00\x00\x00\xf0?\n\x81\x05\n\nconditions\x12\xf2\x042\xef\x04\n^*\\\n\x13\n\x04type\x12\x0b\x1a\tCompleted\n\x10\n\x06status\x12\x06\x1a\x04True\n3\n\x12lastTransitionTime\x12\x1d\x1a\x1b2024-11-21T08:08:15.926525Z\ng*e\n\x1c\n\x04type\x12\x14\x1a\x12ResourcesAvailable\n\x10\n\x06status\x12\x06\x1a\x04True\n3\n\x12lastTransitionTime\x12\x1d\x1a\x1b2024-11-21T08:07:34.746477Z\n\\*Z\n\x11\n\x04type\x12\t\x1a\x07Started\n\x10\n\x06status\x12\x06\x1a\x04True\n3\n\x12lastTransitionTime\x12\x1d\x1a\x1b2024-11-21T08:07:39.015624Z\nc*a\n\x18\n\x04type\x12\x10\x1a\x0eContainerReady\n\x10\n\x06status\x12\x06\x1a\x04True\n3\n\x12lastTransitionTime\x12\x1d\x1a\x1b2024-11-21T08:07:34.648105Z\n\xe0\x01*\xdd\x01\n\x0f\n\x04type\x12\x07\x1a\x05Retry\n\x10\n\x06status\x12\x06\x1a\x04True\n\x1a\n\x06reason\x12\x10\x1a\x0eImmediateRetry\nS\n\x07message\x12H\x1aFSystem will retry after 0:00:00 from lastTransitionTime for attempt 0.\n3\n\x12lastTransitionTime\x12\x1d\x1a\x1b2024-11-21T08:08:16.018236Z\n\x12\n\x08severity\x12\x06\x1a\x04Info\n*\n\tstartTime\x12\x1d\x1a\x1b2024-11-21T08:07:39.015624Z\n/\n\x0ecompletionTime\x12\x1d\x1a\x1b2024-11-21T08:08:15.926525Z\n\x1b\n\x0esucceededCount\x12\t\x11\x00\x00\x00\x00\x00\x00\xf0?\n\xb1\x02\n\x06logUri\x12\xa6\x02\x1a\xa3\x02https://console.cloud.google.com/logs/viewer?project=defang-cd-idhk6xblr21o&advancedFilter=resource.type%3D%22cloud_run_job%22%0Aresource.labels.job_name%3D%22defang-cd%22%0Aresource.labels.location%3D%22us-central1%22%0Alabels.%22run.googleapis.com/execution_name%22%3D%22defang-cd-fqsn2%22\n<\n\x05@type\x123\x1a1type.googleapis.com/google.cloud.run.v1.Execution")},
		},
	}

	resp, err = ParseActivityEntry(jobRunEntry)
	if err != nil {
		t.Fatalf("ParseActivityEntry() failed: %v", err)
	}
	if resp == nil {
		t.Fatal("ParseActivityEntry() returned nil response")
	}
}
