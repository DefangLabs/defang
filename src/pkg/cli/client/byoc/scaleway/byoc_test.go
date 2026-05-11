package scaleway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	cloudscaleway "github.com/DefangLabs/defang/src/pkg/clouds/scaleway"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCdJobNameIsScopedToStack(t *testing.T) {
	t.Parallel()

	client := &ByocScaleway{}
	client.ByocBaseClient = byoc.NewByocBaseClient("", client, "tenant/project/prod")

	name := client.cdJobName()

	assert.Equal(t, "defang-cd-tenant-project-prod", name)
	assert.NotEqual(t, byoc.CdTaskPrefix, name)
}

func TestCdJobNameIsScalewaySafe(t *testing.T) {
	t.Parallel()

	client := &ByocScaleway{}
	client.ByocBaseClient = byoc.NewByocBaseClient("", client, strings.Repeat("stack/", 80))

	name := client.cdJobName()

	assert.LessOrEqual(t, len(name), 255)
	assert.NotContains(t, name, "/")
	assert.NotContains(t, name, "_")
	assert.True(t, strings.HasPrefix(name, byoc.CdTaskPrefix))
}

func TestCdLogQueryUsesScopedJobName(t *testing.T) {
	t.Parallel()

	client := &ByocScaleway{}
	client.ByocBaseClient = byoc.NewByocBaseClient("", client, "tenant/project/prod")

	query := client.buildLogQuery(&defangv1.TailRequest{})

	assert.Equal(t, `{job_definition_name="defang-cd-tenant-project-prod"}`, query)
}

func TestServiceLogQueryUsesServerlessContainerNames(t *testing.T) {
	t.Parallel()

	client := &ByocScaleway{}
	client.ByocBaseClient = byoc.NewByocBaseClient("", client, "tenant/project/prod")

	query := client.buildLogQuery(&defangv1.TailRequest{
		LogType:  uint32(logs.LogTypeAll),
		Services: []string{"app", "api.v1"},
	})

	assert.Equal(t, `{resource_type="serverless_container",resource_name=~".*-(app|api\.v1)"}`, query)
}

func TestEnvironmentIncludesPulumiAndScalewayContext(t *testing.T) {
	t.Setenv("DEFANG_DEBUG", "true")
	t.Setenv("DEFANG_JSON", "true")
	t.Setenv("DEFANG_PULUMI_DEBUG", "1")
	t.Setenv("DEFANG_PULUMI_DIFF", "1")
	t.Setenv("DEFANG_PULUMI_TARGETS", "urn-1,urn-2")

	provider := newTestProvider("tenant/project/prod")
	env, err := provider.environment("demo")
	require.NoError(t, err)

	assert.Equal(t, "access", env["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "secret", env["AWS_SECRET_ACCESS_KEY"])
	assert.Equal(t, "fr-par", env["AWS_REGION"])
	assert.Equal(t, "tenant", env["DEFANG_ORG"])
	assert.Equal(t, "demo.internal", env["PRIVATE_DOMAIN"])
	assert.Equal(t, "demo", env["PROJECT"])
	assert.Equal(t, "tenant/project/prod", env["STACK"])
	assert.Equal(t, "fr-par", env["SCW_DEFAULT_REGION"])
	assert.Equal(t, "project-id", env["SCW_DEFAULT_PROJECT_ID"])
	assert.Equal(t, "access", env["SCW_ACCESS_KEY"])
	assert.Equal(t, "secret", env["SCW_SECRET_KEY"])
	assert.Contains(t, env["DEFANG_STATE_URL"], "s3://defang-cd-tenant/project/prod")
	assert.Contains(t, env["DEFANG_STATE_URL"], "s3.fr-par.scw.cloud")
	assert.Equal(t, "urn-1,urn-2", env["DEFANG_PULUMI_TARGETS"])
}

func TestSecretEnvFiltering(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"AWS_ACCESS_KEY_ID":        "public",
		"AWS_SECRET_ACCESS_KEY":    "aws-secret",
		"PULUMI_CONFIG_PASSPHRASE": "passphrase",
		"SCW_SECRET_KEY":           "scw-secret",
		"SCW_ACCESS_KEY":           "public",
	}

	assert.Equal(t, map[string]string{
		"AWS_SECRET_ACCESS_KEY":    "aws-secret",
		"PULUMI_CONFIG_PASSPHRASE": "passphrase",
		"SCW_SECRET_KEY":           "scw-secret",
	}, cdSecretEnv(env))

	clean := withoutSecretEnv(env)
	assert.Equal(t, "public", clean["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "public", clean["SCW_ACCESS_KEY"])
	assert.NotContains(t, clean, "AWS_SECRET_ACCESS_KEY")
	assert.NotContains(t, clean, "PULUMI_CONFIG_PASSPHRASE")
	assert.NotContains(t, clean, "SCW_SECRET_KEY")
}

func TestUsesScalewayLLMRequiresEndpointAndMissingKey(t *testing.T) {
	endpoint := "https://api.scaleway.ai/v1/"
	key := "already-set"

	assert.True(t, usesScalewayLLM(&compose.Project{Services: compose.Services{
		"chat": {Environment: map[string]*string{
			"OPENAI_BASE_URL": &endpoint,
			"OPENAI_API_KEY":  nil,
		}},
	}}))
	assert.False(t, usesScalewayLLM(&compose.Project{Services: compose.Services{
		"chat": {Environment: map[string]*string{
			"OPENAI_BASE_URL": &endpoint,
			"OPENAI_API_KEY":  &key,
		}},
	}}))
	assert.False(t, usesScalewayLLM(&compose.Project{Services: compose.Services{
		"chat": {Environment: map[string]*string{"OPENAI_API_KEY": nil}},
	}}))
}

func TestDeploymentStatusMapsJobStates(t *testing.T) {
	t.Parallel()

	provider := newTestProvider("prod")
	_, err := provider.GetDeploymentStatus(context.Background())
	require.EqualError(t, err, "no CD run in progress")

	provider.cdRunID = "run-succeeded"
	provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "/serverless-jobs/v1alpha2/regions/fr-par/job-runs/run-succeeded", req.URL.Path)
		return jsonResponse(`{"id":"run-succeeded","state":"succeeded"}`), nil
	})}
	done, err := provider.GetDeploymentStatus(context.Background())
	require.NoError(t, err)
	assert.True(t, done)

	provider.cdRunID = "run-failed"
	provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(`{"id":"run-failed","state":"failed","error_message":"boom"}`), nil
	})}
	done, err = provider.GetDeploymentStatus(context.Background())
	assert.True(t, done)
	var failed client.ErrDeploymentFailed
	require.ErrorAs(t, err, &failed)
	assert.Contains(t, failed.Message, "boom")
}

func TestJobRunStateToServiceState(t *testing.T) {
	t.Parallel()

	assert.Equal(t, defangv1.ServiceState_UPDATE_QUEUED, jobRunStateToServiceState("initialized"))
	assert.Equal(t, defangv1.ServiceState_UPDATE_QUEUED, jobRunStateToServiceState("queued"))
	assert.Equal(t, defangv1.ServiceState_DEPLOYMENT_PENDING, jobRunStateToServiceState("running"))
	assert.Equal(t, defangv1.ServiceState_DEPLOYMENT_COMPLETED, jobRunStateToServiceState("succeeded"))
	assert.Equal(t, defangv1.ServiceState_DEPLOYMENT_FAILED, jobRunStateToServiceState("failed"))
	assert.Equal(t, defangv1.ServiceState_DEPLOYMENT_FAILED, jobRunStateToServiceState("interrupted"))
	assert.Equal(t, defangv1.ServiceState_NOT_SPECIFIED, jobRunStateToServiceState("unknown"))
}

func TestSplitDelegateDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fqdn      string
		domain    string
		subdomain string
	}{
		{fqdn: "example.com", domain: "example.com"},
		{fqdn: "app.example.com.", domain: "example.com", subdomain: "app"},
		{fqdn: "api.staging.example.com", domain: "example.com", subdomain: "api.staging"},
	}
	for _, tt := range tests {
		t.Run(tt.fqdn, func(t *testing.T) {
			t.Parallel()
			domain, subdomain := splitDelegateDomain(tt.fqdn)
			assert.Equal(t, tt.domain, domain)
			assert.Equal(t, tt.subdomain, subdomain)
		})
	}
}

func TestLokiEntryToTailResponse(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 5, 11, 16, 0, 0, 0, time.UTC)
	resp := lokiEntryToTailResponse(cloudscaleway.LokiEntry{
		Timestamp: ts,
		Line:      "ERROR failed to start",
		Labels: map[string]string{
			"resource_name": "app",
			"resource_id":   "container-id",
		},
	}, "etag")

	require.Len(t, resp.Entries, 1)
	assert.Equal(t, "app", resp.Service)
	assert.Equal(t, "etag", resp.Etag)
	assert.Equal(t, "container-id", resp.Entries[0].Host)
	assert.Equal(t, "ERROR failed to start", resp.Entries[0].Message)
	assert.True(t, resp.Entries[0].Stderr)
	assert.True(t, timestamppb.New(ts).AsTime().Equal(resp.Entries[0].Timestamp.AsTime()))

	fallback := lokiEntryToTailResponse(cloudscaleway.LokiEntry{Line: "ok"}, "etag")
	assert.Equal(t, "cd", fallback.Service)
	assert.False(t, fallback.Entries[0].Stderr)
}

func TestLokiEntryToTailResponseParsesScalewayJSONPayload(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 5, 11, 18, 40, 0, 0, time.UTC)
	resp := lokiEntryToTailResponse(cloudscaleway.LokiEntry{
		Timestamp: ts,
		Line:      `{"resource_type":"serverless_job","stream":"stderr","job_definition_name":"defang-cd-logsval","resource_id":"run-id","message":"error: kaniko build failed"}`,
		Labels: map[string]string{
			"resource_type": "serverless_job",
		},
	}, "etag")

	require.Len(t, resp.Entries, 1)
	assert.Equal(t, "defang-cd-logsval", resp.Service)
	assert.Equal(t, "run-id", resp.Entries[0].Host)
	assert.Equal(t, "error: kaniko build failed", resp.Entries[0].Message)
	assert.True(t, resp.Entries[0].Stderr)
}

func TestLokiEntryToTailResponseParsesScalewayRuntimePayload(t *testing.T) {
	t.Parallel()

	resp := lokiEntryToTailResponse(cloudscaleway.LokiEntry{
		Line: `{"resource_instance":"deployment-pod","message":"defang-log-smoke request path=/test-logs","stream":"stdout"}`,
		Labels: map[string]string{
			"resource_name": "scalewaylogsmokef9df5e7b-app",
			"resource_type": "serverless_container",
		},
	}, "etag")

	require.Len(t, resp.Entries, 1)
	assert.Equal(t, "scalewaylogsmokef9df5e7b-app", resp.Service)
	assert.Equal(t, "deployment-pod", resp.Entries[0].Host)
	assert.Equal(t, "defang-log-smoke request path=/test-logs", resp.Entries[0].Message)
	assert.False(t, resp.Entries[0].Stderr)
}

func TestLokiEntryToTailResponseSkipsScalewayMetadataPayload(t *testing.T) {
	t.Parallel()

	resp := lokiEntryToTailResponse(cloudscaleway.LokiEntry{
		Line: `{"resource_type":"serverless_job","stream":"stdout","job_definition_name":"defang-cd-logsval","resource_id":"run-id"}`,
	}, "etag")

	assert.Nil(t, resp)
}

func TestEnsureCockpitTokenIgnoresConcurrentDelete(t *testing.T) {
	t.Parallel()

	provider := newTestProvider("prod")
	createAttempts := 0
	provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cockpit/v1/regions/fr-par/tokens":
			createAttempts++
			if createAttempts == 1 {
				return statusResponse(http.StatusConflict, `{"message":"token already exists"}`), nil
			}
			return jsonResponse(`{"id":"new-token","name":"defang-cd-logs","secret_key":"secret"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/cockpit/v1/regions/fr-par/tokens":
			return jsonResponse(`{"tokens":[{"id":"old-token","name":"defang-cd-logs"}]}`), nil
		case req.Method == http.MethodDelete && req.URL.Path == "/cockpit/v1/regions/fr-par/tokens/old-token":
			return statusResponse(http.StatusNotFound, `{"message":"token was already deleted"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/cockpit/v1/regions/fr-par/data-sources":
			return jsonResponse(`{"data_sources":[{"type":"logs","url":"https://logs.example"}]}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})}

	err := provider.ensureCockpitToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "secret", provider.cockpitToken)
	assert.Equal(t, "https://logs.example", provider.cockpitLogsEndpoint)
}

func TestSubscribeRejectsMissingOrMismatchedRun(t *testing.T) {
	t.Parallel()

	provider := newTestProvider("prod")
	_, err := provider.Subscribe(context.Background(), &defangv1.SubscribeRequest{})
	require.ErrorIs(t, err, errors.ErrUnsupported)

	provider.cdRunID = "run"
	provider.cdEtag = types.ETag("current")
	_, err = provider.Subscribe(context.Background(), &defangv1.SubscribeRequest{Etag: "other"})
	require.ErrorIs(t, err, errors.ErrUnsupported)
}

func TestAccountInfoRequiresAuthAndUsesClientContext(t *testing.T) {
	t.Parallel()

	provider := newTestProvider("prod")
	provider.client = nil
	_, err := provider.AccountInfo(context.Background())
	require.EqualError(t, err, "not authenticated; call Authenticate first")

	provider.client = cloudscaleway.NewClient("access", "secret", "project-id", "fr-par")
	info, err := provider.AccountInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "project-id", info.AccountID)
	assert.Equal(t, client.ProviderScaleway, info.Provider)
	assert.Equal(t, "fr-par", info.Region)
}

func TestConfigLifecycleUsesStackScopedSecretNames(t *testing.T) {
	t.Parallel()

	provider := newTestProvider("prod")
	var requests []string
	provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.Method+" "+req.URL.Path+"?"+req.URL.RawQuery)
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/secret-manager/v1beta1/regions/fr-par/secrets":
			return jsonResponse(`{"id":"secret-id","name":"Defang_demo_prod_API_KEY","project_id":"project-id"}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/secret-manager/v1beta1/regions/fr-par/secrets/secret-id/versions":
			return jsonResponse(`{"secret_id":"secret-id","revision":1}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/secret-manager/v1beta1/regions/fr-par/secrets":
			name := req.URL.Query().Get("name")
			switch name {
			case "":
				return jsonResponse(`{"secrets":[
					{"id":"secret-id","name":"Defang_demo_prod_API_KEY"},
					{"id":"other","name":"Defang_other_prod_API_KEY"}
				]}`), nil
			case "Defang_demo_prod_API_KEY":
				return jsonResponse(`{"secrets":[{"id":"secret-id","name":"Defang_demo_prod_API_KEY"}]}`), nil
			default:
				return jsonResponse(`{"secrets":[]}`), nil
			}
		case req.Method == http.MethodDelete && req.URL.Path == "/secret-manager/v1beta1/regions/fr-par/secrets/secret-id":
			return jsonResponse(`{}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})}

	err := provider.PutConfig(context.Background(), &defangv1.PutConfigRequest{
		Project: "demo",
		Name:    "API_KEY",
		Value:   "value",
	})
	require.NoError(t, err)

	secrets, err := provider.ListConfig(context.Background(), &defangv1.ListConfigsRequest{Project: "demo"})
	require.NoError(t, err)
	assert.Equal(t, []string{"API_KEY"}, secrets.Names)

	err = provider.DeleteConfig(context.Background(), &defangv1.Secrets{Project: "demo", Names: []string{"API_KEY"}})
	require.NoError(t, err)
	assert.Contains(t, requests, "DELETE /secret-manager/v1beta1/regions/fr-par/secrets/secret-id?")
}

func TestRunCdCommandUsesNonSecretEnvironmentOverrides(t *testing.T) {
	t.Parallel()

	provider := newTestProvider("prod")
	provider.jobDefID = "job-def"
	provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "/serverless-jobs/v1alpha2/regions/fr-par/job-definitions/job-def/start", req.URL.Path)
		data, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		body := string(data)
		assert.Contains(t, body, `"startup_command":["/app/cd"]`)
		assert.Contains(t, body, `"args":["preview","payload"]`)
		assert.Contains(t, body, `"DEFANG_ETAG":"etag"`)
		assert.Contains(t, body, `"DOMAIN":"example.com"`)
		assert.Contains(t, body, `"DEFANG_STATES_UPLOAD_URL":"https://states.example"`)
		assert.NotContains(t, body, "SCW_SECRET_KEY")
		assert.NotContains(t, body, "AWS_SECRET_ACCESS_KEY")
		assert.NotContains(t, body, "PULUMI_CONFIG_PASSPHRASE")
		return jsonResponse(`{"job_runs":[{"id":"run-id","state":"queued"}]}`), nil
	})}

	runID, err := provider.runCdCommand(context.Background(), cdCommand{
		command:        []string{"preview", "payload"},
		delegateDomain: "example.com",
		etag:           "etag",
		mode:           defangv1.DeploymentMode_DEVELOPMENT,
		project:        "demo",
		statesUrl:      "https://states.example",
		eventsUrl:      "https://events.example",
	})
	require.NoError(t, err)
	assert.Equal(t, "run-id", runID)
}

func TestCreateCDSecretReferences(t *testing.T) {
	t.Parallel()

	provider := newTestProvider("prod")
	var sawJobSecretRefs bool
	provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/secret-manager/v1beta1/regions/fr-par/secrets":
			data, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			var payload map[string]string
			require.NoError(t, json.Unmarshal(data, &payload))
			return jsonResponse(`{"id":"` + payload["name"] + `","name":"` + payload["name"] + `"}`), nil
		case req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/versions"):
			return jsonResponse(`{"revision":3}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/serverless-jobs/v1alpha2/regions/fr-par/secrets":
			sawJobSecretRefs = true
			data, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			body := string(data)
			assert.Contains(t, body, `"env_var_name":"AWS_SECRET_ACCESS_KEY"`)
			assert.Contains(t, body, `"env_var_name":"PULUMI_CONFIG_PASSPHRASE"`)
			assert.Contains(t, body, `"env_var_name":"SCW_SECRET_KEY"`)
			assert.NotContains(t, body, "SCW_ACCESS_KEY")
			return jsonResponse(`{}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})}

	err := provider.createCDSecretReferences(context.Background(), "job-def", map[string]string{
		"AWS_ACCESS_KEY_ID":        "public",
		"AWS_SECRET_ACCESS_KEY":    "aws-secret",
		"PULUMI_CONFIG_PASSPHRASE": "passphrase",
		"SCW_ACCESS_KEY":           "public",
		"SCW_SECRET_KEY":           "scw-secret",
	})
	require.NoError(t, err)
	assert.True(t, sawJobSecretRefs)
}

func newTestProvider(stack string) *ByocScaleway {
	provider := &ByocScaleway{
		client:    cloudscaleway.NewClient("access", "secret", "project-id", "fr-par"),
		projectID: "project-id",
		region:    "fr-par",
	}
	provider.ByocBaseClient = byoc.NewByocBaseClient("tenant", provider, stack)
	return provider
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(body string) *http.Response {
	return statusResponse(http.StatusOK, body)
}

func statusResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}
