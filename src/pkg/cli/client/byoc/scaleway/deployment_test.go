package scaleway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	cloudscaleway "github.com/DefangLabs/defang/src/pkg/clouds/scaleway"
	"github.com/DefangLabs/defang/src/pkg/deployment"
	"github.com/DefangLabs/defang/src/pkg/logs"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestSubscribeKeepsObservingServicesAfterCDSucceeds(t *testing.T) {
	for _, terminal := range []string{"ready", "error"} {
		t.Run(terminal, func(t *testing.T) {
			t.Parallel()
			provider := newTestProvider("prod")
			provider.cdRunID, provider.cdEtag = "run-id", "attempt"
			metadata := testDeployment("actual-uuid")
			metadata.Summary = false // A component can be available before the project summary.
			serveDeployment(t, provider, metadata)
			var polls int
			provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/serverless-jobs/v1alpha2/regions/fr-par/job-runs/run-id" {
					return jsonResponse(`{"id":"run-id","state":"succeeded"}`), nil
				}
				assert.Equal(t, "/containers/v1beta1/regions/fr-par/containers/actual-uuid", req.URL.Path)
				polls++
				if polls == 1 {
					return jsonResponse(`{"id":"actual-uuid","status":"pending"}`), nil
				}
				return jsonResponse(`{"id":"actual-uuid","status":"` + terminal + `","error_message":"image was not found"}`), nil
			})}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			sequence, err := provider.Subscribe(ctx, &defangv1.SubscribeRequest{Project: "demo", Etag: "attempt", Services: []string{"app"}})
			require.NoError(t, err)
			var events []*defangv1.SubscribeResponse
			for event, err := range sequence {
				require.NoError(t, err)
				events = append(events, event)
			}
			require.Len(t, events, 2)
			assert.Equal(t, "app", events[0].Name)
			assert.Equal(t, defangv1.ServiceState_DEPLOYMENT_PENDING, events[0].State)
			want := defangv1.ServiceState_DEPLOYMENT_COMPLETED
			if terminal == "error" {
				want = defangv1.ServiceState_DEPLOYMENT_FAILED
				assert.Contains(t, events[1].Status, "image was not found")
			}
			assert.Equal(t, want, events[1].State)
		})
	}
}

func TestSubscribeWaitsForEmptyStackBootstrap(t *testing.T) {
	t.Parallel()
	provider := newTestProvider("prod")
	provider.cdRunID, provider.cdEtag = "run-id", "attempt"
	encoded, err := testDeployment("actual-uuid").Encode()
	require.NoError(t, err)
	state, err := json.Marshal(map[string]any{"version": 3, "deployment": map[string]any{
		"resources": []any{map[string]any{"outputs": map[string]any{deployment.OutputName: encoded}}},
	}})
	require.NoError(t, err)
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/defang-cd-prod/.pulumi/stacks/demo/prod.json" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if reads.Add(1) == 1 {
			_, _ = fmt.Fprint(w, `{"version":0,"checkpoint":{"stack":"organization/demo/prod"}}`)
			return
		}
		_, _ = w.Write(state)
	}))
	t.Cleanup(server.Close)
	provider.s3Client = s3.New(s3.Options{Region: "fr-par", BaseEndpoint: aws.String(server.URL), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider("access", "secret", "")})
	provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/serverless-jobs/v1alpha2/regions/fr-par/job-runs/run-id" {
			return jsonResponse(`{"id":"run-id","state":"succeeded"}`), nil
		}
		assert.Equal(t, "/containers/v1beta1/regions/fr-par/containers/actual-uuid", req.URL.Path)
		return jsonResponse(`{"id":"actual-uuid","status":"ready"}`), nil
	})}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	sequence, err := provider.Subscribe(ctx, &defangv1.SubscribeRequest{
		Project: "demo", Etag: "attempt", Services: []string{"app"},
	})
	require.NoError(t, err)
	var events []*defangv1.SubscribeResponse
	for event, err := range sequence {
		require.NoError(t, err)
		events = append(events, event)
	}
	assert.Equal(t, int32(2), reads.Load())
	require.Len(t, events, 1)
	assert.Equal(t, "app", events[0].Name)
	assert.Equal(t, defangv1.ServiceState_DEPLOYMENT_COMPLETED, events[0].State)
}

func TestSubscribeDoesNotTreatOldMetadataOrCDNameAsServiceSuccess(t *testing.T) {
	t.Parallel()
	provider := newTestProvider("prod")
	provider.cdRunID, provider.cdEtag = "run-id", "new-attempt"
	serveDeployment(t, provider, testDeployment("old-id"))
	provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "/serverless-jobs/v1alpha2/regions/fr-par/job-runs/run-id", req.URL.Path)
		return jsonResponse(`{"id":"run-id","state":"succeeded"}`), nil
	})}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	sequence, err := provider.Subscribe(ctx, &defangv1.SubscribeRequest{Project: "demo", Etag: "new-attempt", Services: []string{"cd"}})
	require.NoError(t, err)
	var observedError error
	for event, err := range sequence {
		assert.Nil(t, event, "a CD job update must not satisfy a logical service named cd")
		observedError = err
	}
	require.ErrorIs(t, observedError, context.DeadlineExceeded)
}

func serveDeployment(t *testing.T, provider *ByocScaleway, metadata deployment.Metadata) {
	t.Helper()
	encoded, err := metadata.Encode()
	require.NoError(t, err)
	data, err := json.Marshal(map[string]any{"version": 3, "checkpoint": map[string]any{"latest": map[string]any{
		"resources": []any{map[string]any{"outputs": map[string]any{deployment.OutputName: encoded}}},
	}}})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/defang-cd-prod/.pulumi/stacks/demo/prod.json" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)
	provider.s3Client = s3.New(s3.Options{Region: "fr-par", BaseEndpoint: aws.String(server.URL), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider("access", "secret", "")})
}

func testDeployment(id string) deployment.Metadata {
	return deployment.Metadata{Version: deployment.Version, Provider: "scaleway", Project: "demo", Stack: "prod", Etag: "attempt", Summary: true,
		Services: map[string]deployment.Service{"app": {Kind: "container", ID: "fr-par/" + id, Name: "unpredictable-physical-name", Endpoint: "https://app.example",
			LogLabels: map[string]string{"resource_type": "serverless_container", "resource_id": id}}}}
}

func TestGetServicesUsesPhysicalIDAndObservedStatus(t *testing.T) {
	t.Parallel()
	provider := newTestProvider("prod")
	metadata := testDeployment("actual-uuid")
	app := metadata.Services["app"]
	app.HealthcheckPath = "/health"
	metadata.Services["app"] = app
	metadata.Services["db"] = deployment.Service{Kind: "postgres", ID: "fr-par/db-id", Name: "db"}
	serveDeployment(t, provider, metadata)
	provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "/containers/v1beta1/regions/fr-par/containers/actual-uuid", req.URL.Path)
		return jsonResponse(`{"id":"actual-uuid","name":"unpredictable-physical-name","status":"error"}`), nil
	})}
	response, err := provider.GetServices(t.Context(), &defangv1.GetServicesRequest{Project: "demo"})
	require.NoError(t, err)
	require.Len(t, response.Services, 2)
	assert.Equal(t, "app", response.Services[0].Service.Name)
	assert.Equal(t, "error", response.Services[0].Status)
	assert.Equal(t, defangv1.ServiceState_DEPLOYMENT_FAILED, response.Services[0].State)
	assert.Equal(t, []string{"https://app.example"}, response.Services[0].Endpoints)
	assert.Equal(t, "app.example", response.Services[0].PublicFqdn)
	assert.Equal(t, "/health", response.Services[0].HealthcheckPath)
	assert.Equal(t, "unknown", response.Services[1].Status)
	assert.Equal(t, defangv1.ServiceState_NOT_SPECIFIED, response.Services[1].State)
}

func TestFollowLogsReadsStartupBeforeMetadataAppears(t *testing.T) {
	t.Parallel()
	provider := newTestProvider("prod")
	metadata := testDeployment("container-id")
	metadata.Summary = false
	encoded, err := metadata.Encode()
	require.NoError(t, err)
	var available atomic.Bool
	stateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/defang-cd-prod/.pulumi/stacks/demo/prod.json" {
			http.NotFound(w, req)
			return
		}
		resources := []any{}
		if available.Load() {
			resources = append(resources, map[string]any{"outputs": map[string]any{deployment.OutputName: encoded}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"version": 3, "deployment": map[string]any{"resources": resources}})
	}))
	defer stateServer.Close()
	provider.s3Client = s3.New(s3.Options{Region: "fr-par", BaseEndpoint: aws.String(stateServer.URL), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider("access", "secret", "")})
	initial := time.Now().Add(-time.Minute)
	startup := initial.Add(10 * time.Second)
	cdTime := initial.Add(20 * time.Second)
	logsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		timestamp, line, labels := cdTime, "deploying", map[string]string{"job_definition_name": "defang-cd-prod"}
		if req.URL.Query().Get("query") == `{resource_id="container-id",resource_type="serverless_container"}` {
			assert.LessOrEqual(t, req.URL.Query().Get("start"), strconv.FormatInt(startup.UnixNano(), 10))
			timestamp, line, labels = startup, "startup", map[string]string{"resource_id": "container-id"}
		} else {
			available.Store(true)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
			"result": []any{map[string]any{"stream": labels, "values": [][]string{{strconv.FormatInt(timestamp.UnixNano(), 10), line}}}},
		}})
	}))
	defer logsServer.Close()
	provider.cockpitToken, provider.cockpitLogsEndpoint = "secret", logsServer.URL
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stream, err := provider.QueryLogs(ctx, &defangv1.TailRequest{Project: "demo", Etag: "attempt", Follow: true,
		LogType: uint32(logs.LogTypeAll), Since: timestamppb.New(initial)})
	require.NoError(t, err)
	for response, err := range stream {
		require.NoError(t, err)
		if response.Entries[0].Message == "startup" {
			assert.Equal(t, "app", response.Service)
			return
		}
	}
	t.Fatal("startup log was lost when metadata appeared")
}

func TestLogQueriesDoNotMixProjectsWithSameServiceName(t *testing.T) {
	t.Parallel()
	provider := newTestProvider("prod")
	first, err := provider.buildLogQueries(&defangv1.TailRequest{LogType: uint32(logs.LogTypeRun)}, ptrMetadata(testDeployment("project-one-id")))
	require.NoError(t, err)
	second, err := provider.buildLogQueries(&defangv1.TailRequest{LogType: uint32(logs.LogTypeRun)}, ptrMetadata(testDeployment("project-two-id")))
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
	assert.NotContains(t, first[0], "resource_name")
	_, err = provider.buildLogQueries(&defangv1.TailRequest{Services: []string{"typo"}}, ptrMetadata(testDeployment("id")))
	require.Error(t, err)
	all, err := provider.buildLogQueries(&defangv1.TailRequest{LogType: uint32(logs.LogTypeAll)}, ptrMetadata(testDeployment("project-one-id")))
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, `{job_definition_name="defang-cd-prod"}`, all[1])
	assert.NotContains(t, all[1], "=~")
	_, err = provider.buildLogQueries(&defangv1.TailRequest{LogType: uint32(logs.LogTypeBuild)}, nil)
	require.ErrorContains(t, err, "Scaleway Cockpit")
}

func ptrMetadata(m deployment.Metadata) *deployment.Metadata { return &m }

func TestLogQueriesHandleBuildAliases(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		services []string
		logType  logs.LogType
		wantErr  string
		wantLen  int
	}{
		{"all includes build alias", []string{"app", "app-image"}, logs.LogTypeAll, "", 2},
		{"standalone build alias", []string{"app-image"}, logs.LogTypeAll, "not found", 0},
		{"unknown build alias", []string{"missing-image"}, logs.LogTypeAll, "not found", 0},
		{"run does not accept build alias", []string{"app-image"}, logs.LogTypeRun, "not found", 0},
		{"build gives supported guidance", []string{"app-image"}, logs.LogTypeBuild, "Scaleway Cockpit", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			provider := newTestProvider("prod")
			queries, err := provider.buildLogQueries(&defangv1.TailRequest{
				Services: tt.services, LogType: uint32(tt.logType),
			}, ptrMetadata(testDeployment("id")))
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, queries, tt.wantLen)
		})
	}
	provider := newTestProvider("prod")
	metadata := testDeployment("id")
	metadata.Services["app-image"] = deployment.Service{Name: "app-image", LogLabels: map[string]string{"resource_id": "real-image-service"}}
	queries, err := provider.buildLogQueries(&defangv1.TailRequest{
		Services: []string{"app-image"}, LogType: uint32(logs.LogTypeAll),
	}, &metadata)
	require.NoError(t, err)
	assert.Equal(t, `{resource_id="real-image-service"}`, queries[0])
}

func TestQueryLogsUsesStateAndRestoresComposeServiceName(t *testing.T) {
	t.Parallel()
	provider := newTestProvider("prod")
	serveDeployment(t, provider, testDeployment("actual-uuid"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		assert.Equal(t, `{resource_id="actual-uuid",resource_type="serverless_container"}`, req.URL.Query().Get("query"))
		assert.Equal(t, "Bearer cockpit-secret", req.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"streams","result":[{"stream":{"resource_id":"actual-uuid","resource_name":"unpredictable-physical-name"},"values":[["1700000000000000000","hello"]]}]}}`)
	}))
	defer server.Close()
	provider.cockpitToken, provider.cockpitLogsEndpoint = "cockpit-secret", server.URL
	stream, err := provider.QueryLogs(t.Context(), &defangv1.TailRequest{Project: "demo", LogType: uint32(logs.LogTypeRun)})
	require.NoError(t, err)
	var results []*defangv1.TailResponse
	for response, err := range stream {
		require.NoError(t, err)
		results = append(results, response)
	}
	require.Len(t, results, 1)
	assert.Equal(t, "app", results[0].Service)
	assert.Equal(t, "attempt", results[0].Etag)
	assert.Equal(t, "hello", results[0].Entries[0].Message)
}

func TestReadDeploymentPreservesPermissionErrors(t *testing.T) {
	t.Parallel()
	provider := newTestProvider("prod")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `<Error><Code>AccessDenied</Code><Message>denied</Message></Error>`)
	}))
	defer server.Close()
	provider.s3Client = s3.New(s3.Options{Region: "fr-par", BaseEndpoint: aws.String(server.URL), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider("access", "secret", "")})
	_, err := provider.GetServices(t.Context(), &defangv1.GetServicesRequest{Project: "demo"})
	require.ErrorContains(t, err, "AccessDenied")
}

func TestDeploymentLogResponseUsesPhysicalIdentity(t *testing.T) {
	t.Parallel()
	metadata := testDeployment("container-id")
	response := deploymentLogResponse(cloudscaleway.LokiEntry{Timestamp: time.Now(), Labels: map[string]string{"resource_id": "container-id"}, Line: "message"}, "", &metadata)
	require.NotNil(t, response)
	assert.Equal(t, "app", response.Service)
}
