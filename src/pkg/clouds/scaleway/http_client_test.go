package scaleway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretClientRequests(t *testing.T) {
	t.Parallel()

	var requests []seenRequest
	client := testClient(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, readRequest(t, req))
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/secret-manager/v1beta1/regions/fr-par/secrets":
			return jsonResponse(http.StatusOK, `{"id":"secret-id","name":"app-key","project_id":"project-id"}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/secret-manager/v1beta1/regions/fr-par/secrets/secret-id/versions":
			return jsonResponse(http.StatusOK, `{"secret_id":"secret-id","revision":7}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/secret-manager/v1beta1/regions/fr-par/secrets/secret-id/versions/latest/access":
			return jsonResponse(http.StatusOK, `{"secret_id":"secret-id","revision":7,"data":"dmFsdWU="}`), nil
		case req.Method == http.MethodDelete && req.URL.Path == "/secret-manager/v1beta1/regions/fr-par/secrets/secret-id":
			return jsonResponse(http.StatusNoContent, `{}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	secret, err := client.CreateSecret(context.Background(), "app-key", "")
	require.NoError(t, err)
	assert.Equal(t, "secret-id", secret.ID)

	version, err := client.CreateSecretVersion(context.Background(), secret.ID, []byte("value"))
	require.NoError(t, err)
	assert.Equal(t, 7, version.Revision)

	access, err := client.GetSecretVersion(context.Background(), secret.ID, "latest")
	require.NoError(t, err)
	assert.Equal(t, "dmFsdWU=", access.Data)

	require.NoError(t, client.DeleteSecret(context.Background(), secret.ID))
	require.Len(t, requests, 4)
	assert.Equal(t, "secret", requests[0].authToken)
	assert.JSONEq(t, `{"name":"app-key","project_id":"project-id"}`, requests[0].body)
	assert.JSONEq(t, `{"data":"dmFsdWU=","disable_previous":true}`, requests[1].body)
}

func TestListSecretsFallsBackToClientSidePrefixFiltering(t *testing.T) {
	t.Parallel()

	client := testClient(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "/secret-manager/v1beta1/regions/fr-par/secrets", req.URL.Path)
		if req.URL.Query().Get("name") == "Defang_app_" {
			return jsonResponse(http.StatusOK, `{"secrets":[]}`), nil
		}
		return jsonResponse(http.StatusOK, `{"secrets":[
			{"id":"1","name":"Defang_app_PASSWORD"},
			{"id":"2","name":"Defang_app_TOKEN"},
			{"id":"3","name":"other"}
		]}`), nil
	})

	secrets, err := client.ListSecrets(context.Background(), "", "Defang_app_")
	require.NoError(t, err)
	require.Len(t, secrets, 2)
	assert.Equal(t, "Defang_app_PASSWORD", secrets[0].Name)
	assert.Equal(t, "Defang_app_TOKEN", secrets[1].Name)
}

func TestEnsureSecretValueUsesExistingSecretOnConflict(t *testing.T) {
	t.Parallel()

	client := testClient(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/secret-manager/v1beta1/regions/fr-par/secrets":
			return jsonResponse(http.StatusBadRequest, `{"message":"cannot have same secret name"}`), nil
		case req.Method == http.MethodGet && req.URL.Query().Get("name") == "app-key":
			return jsonResponse(http.StatusOK, `{"secrets":[{"id":"secret-id","name":"app-key"}]}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/secret-manager/v1beta1/regions/fr-par/secrets/secret-id/versions":
			return jsonResponse(http.StatusOK, `{"secret_id":"secret-id","revision":2}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	secret, version, err := client.EnsureSecretValue(context.Background(), "app-key", "", []byte("value"))
	require.NoError(t, err)
	assert.Equal(t, "secret-id", secret.ID)
	assert.Equal(t, 2, version.Revision)
}

func TestJobClientRequests(t *testing.T) {
	t.Parallel()

	client := testClient(func(req *http.Request) (*http.Response, error) {
		body := readRequest(t, req).body
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/serverless-jobs/v1alpha2/regions/fr-par/job-definitions":
			assert.JSONEq(t, `{
				"name":"defang-cd-prod",
				"project_id":"project-id",
				"cpu_limit":1000,
				"memory_limit":2048,
				"local_storage_capacity":5000,
				"image_uri":"image",
				"environment_variables":{"A":"B"}
			}`, body)
			return jsonResponse(http.StatusOK, `{"id":"job-def","name":"defang-cd-prod"}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/serverless-jobs/v1alpha2/regions/fr-par/secrets":
			assert.Contains(t, body, `"env_var_name":"SCW_SECRET_KEY"`)
			return jsonResponse(http.StatusNoContent, `{}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/serverless-jobs/v1alpha2/regions/fr-par/job-definitions/job-def/start":
			assert.Contains(t, body, `"startup_command":["/app/cd"]`)
			assert.Contains(t, body, `"args":["up","payload"]`)
			return jsonResponse(http.StatusOK, `{"job_runs":[{"id":"run-id","state":"queued"}]}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/serverless-jobs/v1alpha2/regions/fr-par/job-runs/run-id":
			return jsonResponse(http.StatusOK, `{"id":"run-id","state":"succeeded"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/serverless-jobs/v1alpha2/regions/fr-par/job-runs":
			assert.Equal(t, "job-def", req.URL.Query().Get("job_definition_id"))
			return jsonResponse(http.StatusOK, `{"job_runs":[{"id":"run-id","state":"succeeded"}]}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/serverless-jobs/v1alpha2/regions/fr-par/job-definitions":
			assert.Equal(t, "defang-cd-prod", req.URL.Query().Get("name"))
			return jsonResponse(http.StatusOK, `{"job_definitions":[{"id":"job-def","name":"defang-cd-prod"}]}`), nil
		case req.Method == http.MethodDelete && req.URL.Path == "/serverless-jobs/v1alpha2/regions/fr-par/job-definitions/job-def":
			return jsonResponse(http.StatusNoContent, `{}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	def, err := client.CreateJobDefinition(context.Background(), "defang-cd-prod", "image", map[string]string{"A": "B"}, JobResources{
		CPULimit:             1000,
		MemoryLimit:          2048,
		LocalStorageCapacity: 5000,
	})
	require.NoError(t, err)
	require.NoError(t, client.CreateJobSecrets(context.Background(), def.ID, []JobSecretRef{{
		SecretManagerID:      "secret-id",
		SecretManagerVersion: "1",
		EnvVarName:           "SCW_SECRET_KEY",
	}}))

	run, err := client.RunJob(context.Background(), def.ID, []string{"/app/cd"}, []string{"up", "payload"}, map[string]string{"A": "B"})
	require.NoError(t, err)
	assert.Equal(t, "run-id", run.ID)

	run, err = client.GetJobRun(context.Background(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, "succeeded", run.State)

	runs, err := client.ListJobRuns(context.Background(), def.ID)
	require.NoError(t, err)
	require.Len(t, runs, 1)

	defs, err := client.ListJobDefinitions(context.Background(), "defang-cd-prod")
	require.NoError(t, err)
	require.Len(t, defs, 1)

	require.NoError(t, client.DeleteJobDefinition(context.Background(), def.ID))
	assert.NoError(t, client.CreateJobSecrets(context.Background(), def.ID, nil))
}

func TestRegistryClientRequests(t *testing.T) {
	t.Parallel()

	client := testClient(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/registry/v1/regions/fr-par/namespaces" && req.URL.Query().Get("name") == "defang-cd":
			return jsonResponse(http.StatusOK, `{"namespaces":[]}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/registry/v1/regions/fr-par/namespaces":
			body := readRequest(t, req).body
			assert.JSONEq(t, `{"name":"defang-cd","project_id":"project-id","is_public":false}`, body)
			return jsonResponse(http.StatusOK, `{"id":"namespace-id","name":"defang-cd","endpoint":"rg.fr-par.scw.cloud/defang-cd"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/registry/v1/regions/fr-par/images":
			assert.Equal(t, "namespace-id", req.URL.Query().Get("namespace_id"))
			return jsonResponse(http.StatusOK, `{"images":[{"id":"image-id","name":"cd","tags":["test"]}]}`), nil
		case req.Method == http.MethodDelete && req.URL.Path == "/registry/v1/regions/fr-par/images/image-id":
			return jsonResponse(http.StatusNoContent, `{}`), nil
		case req.Method == http.MethodDelete && req.URL.Path == "/registry/v1/regions/fr-par/namespaces/namespace-id":
			return jsonResponse(http.StatusNoContent, `{}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	ns, err := client.EnsureRegistryNamespaceExists(context.Background(), "defang-cd", "", "")
	require.NoError(t, err)
	assert.Equal(t, "namespace-id", ns.ID)

	images, err := client.ListImages(context.Background(), ns.ID)
	require.NoError(t, err)
	require.Len(t, images, 1)
	assert.Equal(t, "image-id", images[0].ID)

	require.NoError(t, client.DeleteImage(context.Background(), images[0].ID))
	require.NoError(t, client.DeleteRegistryNamespace(context.Background(), ns.ID))
}

func TestDNSClientRequests(t *testing.T) {
	t.Parallel()

	client := testClient(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/domain/v2beta1/dns-zones":
			assert.JSONEq(t, `{"domain":"example.com","subdomain":"app","project_id":"project-id"}`, readRequest(t, req).body)
			return jsonResponse(http.StatusOK, `{"domain":"example.com","subdomain":"app","ns":["ns0.scaleway.com"]}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/domain/v2beta1/dns-zones":
			assert.Equal(t, "app.example.com", req.URL.Query().Get("domain"))
			return jsonResponse(http.StatusOK, `{"dns_zones":[{"domain":"example.com","subdomain":"app","ns":["ns0.scaleway.com"]}]}`), nil
		case req.Method == http.MethodDelete && req.URL.Path == "/domain/v2beta1/dns-zones/app.example.com":
			return jsonResponse(http.StatusNoContent, `{}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	zone, err := client.CreateDNSZone(context.Background(), "example.com", "app", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"ns0.scaleway.com"}, zone.NS)

	zone, err = client.GetDNSZone(context.Background(), "app.example.com")
	require.NoError(t, err)
	assert.Equal(t, "app", zone.Subdomain)

	require.NoError(t, client.DeleteDNSZone(context.Background(), "app.example.com"))
}

func TestCockpitClientRequests(t *testing.T) {
	t.Parallel()

	client := testClient(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cockpit/v1/regions/fr-par/tokens":
			assert.JSONEq(t, `{"name":"defang-cd-logs","project_id":"project-id","token_scopes":["read_only_logs"]}`, readRequest(t, req).body)
			return jsonResponse(http.StatusOK, `{"id":"token-id","name":"defang-cd-logs","secret_key":"cockpit-secret"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/cockpit/v1/regions/fr-par/tokens":
			assert.Equal(t, "project-id", req.URL.Query().Get("project_id"))
			return jsonResponse(http.StatusOK, `{"tokens":[{"id":"token-id","name":"defang-cd-logs"}]}`), nil
		case req.Method == http.MethodDelete && req.URL.Path == "/cockpit/v1/regions/fr-par/tokens/token-id":
			return jsonResponse(http.StatusNoContent, `{}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/cockpit/v1/regions/fr-par/data-sources":
			assert.Equal(t, "project-id", req.URL.Query().Get("project_id"))
			return jsonResponse(http.StatusOK, `{"data_sources":[
				{"id":"metrics","type":"metrics","url":"https://metrics.example"},
				{"id":"logs","type":"logs","url":"https://logs.example"}
			]}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	token, err := client.CreateCockpitToken(context.Background(), "defang-cd-logs", "")
	require.NoError(t, err)
	assert.Equal(t, "cockpit-secret", token.SecretKey)

	tokens, err := client.ListCockpitTokens(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, tokens, 1)

	endpoint, err := client.GetCockpitLogsEndpoint(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "https://logs.example", endpoint)

	require.NoError(t, client.DeleteCockpitToken(context.Background(), "token-id"))
	assert.Equal(t, "https://logs.cockpit.fr-par.scw.cloud", CockpitLogsEndpoint("fr-par"))
}

func TestAuthenticateAndNewClientFromEnv(t *testing.T) {
	t.Setenv("SCW_ACCESS_KEY", "access")
	t.Setenv("SCW_SECRET_KEY", "secret")
	t.Setenv("SCW_DEFAULT_PROJECT_ID", "project-id")
	t.Setenv("SCW_DEFAULT_ORGANIZATION_ID", "org-id")
	t.Setenv("SCW_DEFAULT_REGION", "")

	client, err := NewClientFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "fr-par", client.Region)
	assert.Equal(t, "org-id", client.OrganizationID)

	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "/iam/v1alpha1/api-keys/access", req.URL.Path)
		return jsonResponse(http.StatusOK, `{"access_key":"access","default_project_id":"project-id","organization_id":"org-from-api"}`), nil
	})}

	info, err := client.Authenticate(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "org-from-api", info.OrganizationID)
	assert.Equal(t, "org-from-api", client.OrganizationID)
	assert.Equal(t, "project-id", client.GetAccountInfo().ProjectID)
}

func TestQueryLokiParsesEntriesAndSkipsMalformedValues(t *testing.T) {
	originalClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = originalClient })

	start := time.Unix(10, 0)
	end := time.Unix(20, 0)
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "Bearer cockpit-secret", req.Header.Get("Authorization"))
		assert.Equal(t, `{resource_name="app"}`, req.URL.Query().Get("query"))
		assert.Equal(t, "25", req.URL.Query().Get("limit"))
		assert.Equal(t, start.UnixNano(), mustParseInt64(t, req.URL.Query().Get("start")))
		assert.Equal(t, end.UnixNano(), mustParseInt64(t, req.URL.Query().Get("end")))
		return jsonResponse(http.StatusOK, `{
			"status":"success",
			"data":{"resultType":"streams","result":[{
				"stream":{"resource_name":"app","resource_id":"container-id"},
				"values":[["1000000000","line one"],["bad","skip"],["2000000000"]]
			}]}
		}`), nil
	})}

	entries, err := QueryLoki(context.Background(), "cockpit-secret", "https://logs.example", `{resource_name="app"}`, start, end, 25)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "line one", entries[0].Line)
	assert.Equal(t, "app", entries[0].Labels["resource_name"])
	assert.Equal(t, time.Unix(0, 1000000000), entries[0].Timestamp)
}

type seenRequest struct {
	authToken string
	body      string
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testClient(fn roundTripFunc) *Client {
	client := NewClient("access", "secret", "project-id", "fr-par")
	client.HTTPClient = &http.Client{Transport: fn}
	return client
}

func readRequest(t *testing.T, req *http.Request) seenRequest {
	t.Helper()
	var body string
	if req.Body != nil {
		data, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		body = string(data)
	}
	if body != "" {
		var js any
		require.NoError(t, json.Unmarshal([]byte(body), &js))
	}
	return seenRequest{
		authToken: req.Header.Get("X-Auth-Token"),
		body:      body,
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

func mustParseInt64(t *testing.T, value string) int64 {
	t.Helper()
	var parsed int64
	_, err := fmt.Sscan(value, &parsed)
	require.NoError(t, err)
	return parsed
}
