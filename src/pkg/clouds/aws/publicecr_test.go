package aws

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	defanghttp "github.com/DefangLabs/defang/src/pkg/http"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecrpublic"
	"github.com/aws/aws-sdk-go-v2/service/ecrpublic/types"
	"github.com/aws/smithy-go/ptr"
)

type MockPublicECRClient struct{}

func (m MockPublicECRClient) GetAuthorizationToken(ctx context.Context, params *ecrpublic.GetAuthorizationTokenInput, optFns ...func(*ecrpublic.Options)) (*ecrpublic.GetAuthorizationTokenOutput, error) {
	return &ecrpublic.GetAuthorizationTokenOutput{
		AuthorizationData: &types.AuthorizationData{
			AuthorizationToken: ptr.String("mocked-token"),
		},
	}, nil
}

type MockHTTPRoundTripper struct {
	StatusCode int
	Body       string
}

func (m *MockHTTPRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("Authorization") != "Bearer mocked-token" {
		return nil, errors.New("missing or incorrect authorization header")
	}

	resp := &http.Response{
		StatusCode: m.StatusCode,
		Body:       io.NopCloser(strings.NewReader(m.Body)),
		Header:     make(http.Header),
	}
	return resp, nil
}

func TestCheckImageExistOnPublicECR(t *testing.T) {
	tests := []struct {
		name           string
		repo           string
		tag            string
		mockStatusCode int
		mockBody       string
		expectedExists bool
		expectedError  string
	}{
		{
			name:           "Image exists",
			repo:           "public.ecr.aws/mock/repo",
			tag:            "latest",
			mockStatusCode: 200,
			expectedExists: true,
		},
		{
			name:           "Image does not exist",
			repo:           "public.ecr.aws/mock/repo",
			tag:            "nonexistent",
			mockStatusCode: 404,
			expectedExists: false,
		},
		{
			name:           "Throttled",
			repo:           "public.ecr.aws/mock/repo",
			tag:            "latest",
			mockStatusCode: 429,
			mockBody:       "throttled",
			expectedExists: false,
			expectedError:  "unexpected status 429: throttled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldHttpClient := defanghttp.DefaultClient
			oldPublicECRClient := newPublicECRClientFromConfig
			oldStsFromConfig := NewStsFromConfig
			defer func() {
				defanghttp.DefaultClient = oldHttpClient
				newPublicECRClientFromConfig = oldPublicECRClient
				NewStsFromConfig = oldStsFromConfig
			}()
			NewStsFromConfig = func(cfg aws.Config) StsClientAPI { return &MockStsClientAPI{} }
			defanghttp.DefaultClient = &http.Client{Transport: &MockHTTPRoundTripper{StatusCode: tt.mockStatusCode, Body: tt.mockBody}}
			newPublicECRClientFromConfig = func(cfg aws.Config) PublicECRAPI { return MockPublicECRClient{} }

			exists, err := CheckImageExistOnPublicECR(t.Context(), tt.repo, tt.tag)
			if err != nil {
				if tt.expectedError == "" {
					t.Fatalf("unexpected error: %v", err)
				} else if err.Error() != tt.expectedError {
					t.Fatalf("expected error to be %q, got %v", tt.expectedError, err)
				}
			}
			if exists != tt.expectedExists {
				t.Fatalf("expected exists to be %v, got %v", tt.expectedExists, exists)
			}
		})
	}
}
