package scaleway

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func setupLifecycle(t *testing.T, provider *ByocScaleway) *[]string {
	t.Helper()
	provider.CDImage = "example/cd@sha256:branch-digest"
	var requests []string
	provider.s3Client = s3.New(s3.Options{Region: "fr-par", BaseEndpoint: aws.String("https://storage.example"), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""),
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, http.MethodHead, req.Method)
			assert.Equal(t, "/defang-cd-prod", req.URL.Path)
			return jsonResponse(""), nil
		})}})
	provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.Method+" "+req.URL.Path)
		switch {
		case strings.HasSuffix(req.URL.Path, "/namespaces"):
			return jsonResponse(`{"namespaces":[{"id":"registry-id","name":"defang-cd","endpoint":"registry.example/defang-cd"}]}`), nil
		case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/job-definitions"):
			assert.Equal(t, "defang-cd-prod", req.URL.Query().Get("name"))
			return jsonResponse(`{"job_definitions":[{"id":"old1","name":"defang-cd-prod"},{"id":"old2","name":"defang-cd-prod"},{"id":"unrelated","name":"defang-cd-other"}]}`), nil
		case req.Method == http.MethodDelete:
			assert.NotContains(t, req.URL.Path, "unrelated")
			return jsonResponse(`{}`), nil
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/job-definitions"):
			var body struct {
				Image string            `json:"image_uri"`
				Env   map[string]string `json:"environment_variables"`
			}
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			assert.Equal(t, provider.CDImage, body.Image)
			assert.NotContains(t, body.Env, "SCW_SECRET_KEY")
			assert.NotContains(t, body.Env, "AWS_SECRET_ACCESS_KEY")
			assert.NotContains(t, body.Env, "PULUMI_CONFIG_PASSPHRASE")
			assert.Equal(t, "fr-par", body.Env["SCW_DEFAULT_REGION"])
			return jsonResponse(`{"id":"new-job","name":"defang-cd-prod"}`), nil
		case strings.Contains(req.URL.Path, "/secret-manager/"):
			if strings.HasSuffix(req.URL.Path, "/versions") {
				return jsonResponse(`{"revision":1}`), nil
			}
			return jsonResponse(`{"id":"cd-secret"}`), nil
		case strings.HasSuffix(req.URL.Path, "/secrets"):
			var body struct {
				Secrets []struct {
					Env string `json:"env_var_name"`
				} `json:"secrets"`
			}
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			require.NotEmpty(t, body.Secrets)
			return jsonResponse(`{}`), nil
		default:
			t.Fatalf("unexpected setup request %s %s", req.Method, req.URL)
			return nil, nil
		}
	})}
	return &requests
}

func TestCDSetupUsesPinnedImageAndSecretReferences(t *testing.T) {
	t.Setenv("DEFANG_PULUMI_DIR", "")
	provider := newTestProvider("prod")
	requests := setupLifecycle(t, provider)
	require.NoError(t, provider.SetUpCD(t.Context(), false))
	assert.True(t, provider.SetupDone)
	assert.Equal(t, "new-job", provider.jobDefID)
	assert.Contains(t, *requests, "DELETE /serverless-jobs/v1alpha2/regions/fr-par/job-definitions/old1")
	assert.Contains(t, *requests, "DELETE /serverless-jobs/v1alpha2/regions/fr-par/job-definitions/old2")
	count := len(*requests)
	require.NoError(t, provider.SetUpCD(t.Context(), false))
	assert.Len(t, *requests, count, "a configured client must not recreate its running job")
}

func TestDeployAndPreviewPassPayloadToCD(t *testing.T) {
	for _, preview := range []bool{false, true} {
		t.Run(map[bool]string{false: "up", true: "preview"}[preview], func(t *testing.T) {
			provider := newTestProvider("prod")
			recipe := &defangv1.Recipe{Name: "affordable", PulumiConfig: "config:\n  defang-scaleway:log-retention-days: 2\n"}
			provider.SetupDone = true
			provider.jobDefID = "existing-job"
			provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "/serverless-jobs/v1alpha2/regions/fr-par/job-definitions/existing-job/start", req.URL.Path)
				var body struct {
					Args []string          `json:"args"`
					Env  map[string]string `json:"environment_variables"`
				}
				require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
				require.Len(t, body.Args, 2)
				assert.Equal(t, map[bool]string{false: "up", true: "preview"}[preview], body.Args[0])
				payload, err := base64.StdEncoding.DecodeString(body.Args[1])
				require.NoError(t, err)
				var update defangv1.ProjectUpdate
				require.NoError(t, proto.Unmarshal(payload, &update))
				assert.True(t, proto.Equal(recipe, update.Recipe), "CD must receive the selected recipe")
				assert.Equal(t, "app", update.Services[0].Service.Name)
				assert.Equal(t, update.Etag, body.Env["DEFANG_ETAG"])
				assert.Equal(t, "demo", body.Env["PROJECT"])
				assert.NotContains(t, body.Env, "SCW_SECRET_KEY")
				return jsonResponse(`{"job_runs":[{"id":"run-id","state":"queued"}]}`), nil
			})}
			req := &client.DeployRequest{DeployRequest: defangv1.DeployRequest{Compose: []byte("name: demo\nservices:\n  app:\n    image: nginx:alpine\n")}}
			req.Recipe = recipe
			var response *client.DeployResponse
			var err error
			if preview {
				response, err = provider.Preview(t.Context(), req)
			} else {
				response, err = provider.Deploy(t.Context(), req)
			}
			require.NoError(t, err)
			assert.Equal(t, "run-id", response.CdId)
			assert.Equal(t, response.Etag, provider.cdEtag)
			assert.Equal(t, "run-id", provider.cdRunID)
		})
	}
}

func TestLegacyProjectReadbackAndServiceLookup(t *testing.T) {
	t.Parallel()
	provider := newTestProvider("prod")
	update := &defangv1.ProjectUpdate{Project: "demo", Services: []*defangv1.ServiceInfo{{Service: &defangv1.Service{Name: "app"}, Status: "recorded"}}}
	data, err := proto.Marshal(update)
	require.NoError(t, err)
	provider.s3Client = s3.New(s3.Options{Region: "fr-par", BaseEndpoint: aws.String("https://storage.example"), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""),
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, ".pulumi/") {
				return statusResponse(http.StatusNotFound, `<Error><Code>NoSuchKey</Code></Error>`), nil
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
		})}})
	service, err := provider.GetService(t.Context(), &defangv1.GetRequest{Project: "demo", Name: "app"})
	require.NoError(t, err)
	assert.Equal(t, "recorded", service.Status)
	_, err = provider.GetService(t.Context(), &defangv1.GetRequest{Project: "demo", Name: "missing"})
	require.ErrorContains(t, err, "not found")
}

func TestCDTeardownFiltersUnrelatedResourcesAndCollectsFailures(t *testing.T) {
	for _, tc := range []struct {
		name         string
		cached, fail bool
	}{
		{name: "fresh client"},
		{name: "cached client", cached: true},
		{name: "fresh client errors", fail: true},
		{name: "cached client errors", cached: true, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// TearDownCD writes to the process-wide terminal warning collector.
			provider := newTestProvider("prod")
			if tc.cached {
				provider.jobDefID = "current-job"
			}
			var deleted []string
			provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodDelete {
					assert.NotContains(t, req.URL.Path, "unrelated")
					deleted = append(deleted, req.URL.Path)
					if tc.fail {
						return statusResponse(http.StatusForbidden, `{"message":"denied"}`), nil
					}
					return jsonResponse(`{}`), nil
				}
				switch {
				case strings.HasSuffix(req.URL.Path, "/job-definitions"):
					return jsonResponse(`{"job_definitions":[{"id":"legacy-job","name":"defang-cd"},{"id":"current-job","name":"defang-cd-prod"},{"id":"unrelated-stack","name":"defang-cd-other"}]}`), nil
				case strings.HasSuffix(req.URL.Path, "/tokens"):
					return jsonResponse(`{"tokens":[{"id":"cd-token","name":"defang-cd-prod-logs"},{"id":"unrelated-token","name":"defang-cd-other-logs"}]}`), nil
				case strings.Contains(req.URL.Path, "/secret-manager/"):
					prefix := provider.cdSecretName("")
					return jsonResponse(`{"secrets":[{"id":"cd-secret","name":"` + prefix + `SCW_SECRET_KEY"},{"id":"unrelated","name":"app-config"}]}`), nil
				case strings.HasSuffix(req.URL.Path, "/namespaces"):
					return jsonResponse(`{"namespaces":[{"id":"cd-registry","name":"defang-cd"},{"id":"unrelated","name":"app-images"}]}`), nil
				case strings.HasSuffix(req.URL.Path, "/images"):
					assert.Equal(t, "cd-registry", req.URL.Query().Get("namespace_id"))
					return jsonResponse(`{"images":[{"id":"cd-image"}]}`), nil
				default:
					t.Fatalf("unexpected teardown request %s %s", req.Method, req.URL)
					return nil, nil
				}
			})}
			provider.s3Client = s3.New(s3.Options{Region: "fr-par", BaseEndpoint: aws.String("https://storage.example"), UsePathStyle: true,
				Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""),
				HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if req.Method == http.MethodGet {
						return jsonResponse(`<ListBucketResult/>`), nil
					}
					deleted = append(deleted, "bucket")
					return jsonResponse(""), nil
				})}})
			err := provider.TearDownCD(t.Context())
			if tc.fail {
				require.ErrorContains(t, err, "denied")
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, deleted, 7, "cleanup should continue through independent resources even after a failure")
			assert.Contains(t, deleted, "/serverless-jobs/v1alpha2/regions/fr-par/job-definitions/current-job")
			assert.Contains(t, deleted, "/cockpit/v1/regions/fr-par/tokens/cd-token")
		})
	}
}

func TestCdListSkipsInvalidObjectsAndLoadsState(t *testing.T) {
	t.Parallel()
	provider := newTestProvider("prod")
	provider.s3Client = s3.New(s3.Options{Region: "fr-par", BaseEndpoint: aws.String("https://storage.example"), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""),
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Query().Get("list-type") == "2" {
				assert.Equal(t, ".pulumi/stacks/", req.URL.Query().Get("prefix"))
				return jsonResponse(`<ListBucketResult><Contents><Key>.pulumi/stacks/demo/bad.json</Key></Contents><Contents><Key>.pulumi/stacks/demo/prod.json</Key></Contents></ListBucketResult>`), nil
			}
			if strings.Contains(req.URL.Path, "bad.json") {
				return jsonResponse("invalid json"), nil
			}
			return jsonResponse(`{"version":3,"checkpoint":{"stack":"organization/demo/prod","latest":{"resources":[{}]}}}` + strings.Repeat(" ", 700)), nil
		})}})
	sequence, err := provider.CdList(t.Context(), false)
	require.NoError(t, err)
	var results []state.Info
	for info := range sequence {
		results = append(results, info)
	}
	require.Len(t, results, 1)
	assert.Equal(t, "demo", results[0].Project)
	assert.Equal(t, "prod", results[0].Stack)
	assert.Equal(t, "fr-par", results[0].CdRegion)
}

func TestSubscribeReportsTerminalCDResult(t *testing.T) {
	for _, status := range []string{"succeeded", "failed"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			provider := newTestProvider("prod")
			provider.cdRunID, provider.cdEtag = "run-id", "attempt"
			provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				assert.True(t, strings.HasSuffix(req.URL.Path, "/job-runs/run-id"))
				return jsonResponse(`{"id":"run-id","state":"` + status + `"}`), nil
			})}
			sequence, err := provider.Subscribe(t.Context(), &defangv1.SubscribeRequest{Etag: "attempt"})
			require.NoError(t, err)
			count := 0
			for event, err := range sequence {
				require.NoError(t, err)
				assert.Equal(t, "cd", event.Name)
				assert.Equal(t, status, event.Status)
				count++
			}
			assert.Equal(t, 1, count)
		})
	}
}

func TestDomainDelegationUsesExistingZoneAndFallsBackToNativeDomain(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		existing  bool
		response  string
		wantError bool
	}{
		{name: "create", status: http.StatusOK, response: `{"ns":["ns.example"]}`},
		{name: "existing", status: http.StatusConflict, existing: true, response: `{"dns_zones":[{"ns":["ns.example"]}]}`},
		{name: "native-domain", status: http.StatusForbidden},
		{name: "missing-nameservers", status: http.StatusOK, response: `{}`, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			provider := newTestProvider("prod")
			provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "/domain/v2beta1/dns-zones", req.URL.Path)
				if req.Method == http.MethodGet {
					assert.True(t, test.existing)
					assert.Equal(t, "app.example.com", req.URL.Query().Get("domain"))
					return jsonResponse(test.response), nil
				}
				if test.status != http.StatusOK {
					return statusResponse(test.status, `{"message":"zone unavailable"}`), nil
				}
				return jsonResponse(test.response), nil
			})}
			result, err := provider.PrepareDomainDelegation(t.Context(), client.PrepareDomainDelegationRequest{DelegateDomain: "app.example.com"})
			if test.wantError {
				require.ErrorContains(t, err, "no nameservers")
				return
			}
			require.NoError(t, err)
			if test.status == http.StatusForbidden {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, []string{"ns.example"}, result.NameServers)
			}
		})
	}
}

func TestCdListMissingBucketAndPermissionErrors(t *testing.T) {
	for _, tc := range []struct {
		code   string
		status int
	}{{"NoSuchBucket", http.StatusNotFound}, {"AccessDenied", http.StatusForbidden}} {
		t.Run(tc.code, func(t *testing.T) {
			provider := newTestProvider("prod")
			provider.s3Client = s3.New(s3.Options{Region: "fr-par", BaseEndpoint: aws.String("https://storage.example"), UsePathStyle: true, Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""), HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return statusResponse(tc.status, "<Error><Code>"+tc.code+"</Code></Error>"), nil
			})}})
			sequence, err := provider.CdList(t.Context(), false)
			if tc.code == "AccessDenied" {
				require.ErrorContains(t, err, "AccessDenied")
				return
			}
			require.NoError(t, err)
			for range sequence {
				t.Fatal("missing bucket must have no stacks")
			}
		})
	}
}
