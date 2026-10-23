package scaleway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
)

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
	assert.Equal(t, "unknown", response.Services[1].Status)
	assert.Equal(t, defangv1.ServiceState_NOT_SPECIFIED, response.Services[1].State)
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
}

func ptrMetadata(m deployment.Metadata) *deployment.Metadata { return &m }

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
