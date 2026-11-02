package scaleway

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storageClient(handler func(*http.Request) (*http.Response, error)) *s3.Client {
	return s3.New(s3.Options{Region: "fr-par", BaseEndpoint: aws.String("https://storage.example"), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""), RetryMaxAttempts: 1,
		HTTPClient: &http.Client{Transport: roundTripFunc(handler)}})
}

func storageResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/xml"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestStorageBucketAndObjectRoundTrip(t *testing.T) {
	t.Parallel()
	var exists bool
	var stored string
	client := storageClient(func(req *http.Request) (*http.Response, error) {
		assert.Contains(t, req.Header.Get("Authorization"), "Credential=access/")
		switch req.Method + " " + req.URL.Path {
		case "HEAD /state":
			if exists {
				return storageResponse(http.StatusOK, ""), nil
			}
			return storageResponse(http.StatusNotFound, ""), nil
		case "PUT /state":
			exists = true
		case "PUT /state/.pulumi/stacks/demo/prod.json":
			data, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			stored = string(data)
		case "GET /state/.pulumi/stacks/demo/prod.json":
			return storageResponse(http.StatusOK, stored), nil
		default:
			t.Fatalf("unexpected storage request %s %s", req.Method, req.URL)
		}
		return storageResponse(http.StatusOK, ""), nil
	})
	require.NoError(t, EnsureBucketExists(t.Context(), client, "state", "fr-par"))
	require.NoError(t, EnsureBucketExists(t.Context(), client, "state", "fr-par"))
	require.NoError(t, PutObject(t.Context(), client, "state", ".pulumi/stacks/demo/prod.json", strings.NewReader(`{"version":3}`)))
	data, err := GetObject(t.Context(), client, "state", ".pulumi/stacks/demo/prod.json")
	require.NoError(t, err)
	assert.JSONEq(t, `{"version":3}`, string(data))
	presigned, err := CreatePresignedUploadURL(t.Context(), client, "state", "uploads/artifact", 15*time.Minute)
	require.NoError(t, err)
	parsed, err := url.Parse(presigned)
	require.NoError(t, err)
	assert.Equal(t, "/state/uploads/artifact", parsed.Path)
	assert.Equal(t, "900", parsed.Query().Get("X-Amz-Expires"))
}

func TestStorageCleanupPaginatesAndBatches(t *testing.T) {
	t.Parallel()
	var deleted []string
	var batches []int
	bucketDeleted := false
	client := storageClient(func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodGet:
			assert.Equal(t, "2", req.URL.Query().Get("list-type"))
			if req.URL.Query().Get("continuation-token") == "page2" {
				return storageResponse(http.StatusOK, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>last</Key></Contents></ListBucketResult>`), nil
			}
			var body strings.Builder
			body.WriteString(`<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>page2</NextContinuationToken>`)
			for i := range 1000 {
				fmt.Fprintf(&body, `<Contents><Key>key-%d</Key></Contents>`, i)
			}
			body.WriteString(`</ListBucketResult>`)
			return storageResponse(http.StatusOK, body.String()), nil
		case http.MethodPost:
			var payload struct {
				Objects []struct{ Key string } `xml:"Object"`
			}
			require.NoError(t, xml.NewDecoder(req.Body).Decode(&payload))
			batches = append(batches, len(payload.Objects))
			for _, object := range payload.Objects {
				deleted = append(deleted, object.Key)
			}
			return storageResponse(http.StatusOK, `<DeleteResult/>`), nil
		case http.MethodDelete:
			assert.Len(t, deleted, 1001, "bucket must not be deleted before its contents")
			bucketDeleted = true
			return storageResponse(http.StatusNoContent, ""), nil
		default:
			t.Fatalf("unexpected storage request %s", req.Method)
			return nil, nil
		}
	})
	require.NoError(t, EmptyAndDeleteBucket(t.Context(), client, "state"))
	assert.Equal(t, []int{1000, 1}, batches)
	assert.Equal(t, "last", deleted[1000])
	assert.True(t, bucketDeleted)
}

func TestStoragePermissionFailuresArePreserved(t *testing.T) {
	t.Parallel()
	client := storageClient(func(req *http.Request) (*http.Response, error) {
		return storageResponse(http.StatusForbidden, `<Error><Code>AccessDenied</Code><Message>denied</Message></Error>`), nil
	})
	_, err := GetObject(t.Context(), client, "state", "key")
	require.ErrorContains(t, err, "AccessDenied")
	require.ErrorContains(t, PutObject(t.Context(), client, "state", "key", strings.NewReader("payload")), "AccessDenied")
	_, err = ListObjectKeys(t.Context(), client, "state", ".pulumi/stacks/")
	require.ErrorContains(t, err, "AccessDenied")
	require.ErrorContains(t, EmptyAndDeleteBucket(t.Context(), client, "state"), "AccessDenied")
	require.ErrorContains(t, EnsureBucketExists(t.Context(), client, "state", "fr-par"), "AccessDenied")
}

func TestStorageCleanupAllowsMissingBucket(t *testing.T) {
	for _, deletedDuringCleanup := range []bool{false, true} {
		t.Run(strconv.FormatBool(deletedDuringCleanup), func(t *testing.T) {
			client := storageClient(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && deletedDuringCleanup {
					return storageResponse(http.StatusOK, `<ListBucketResult/>`), nil
				}
				return storageResponse(http.StatusNotFound, `<Error><Code>NoSuchBucket</Code></Error>`), nil
			})
			require.NoError(t, EmptyAndDeleteBucket(t.Context(), client, "state"))
		})
	}
}
