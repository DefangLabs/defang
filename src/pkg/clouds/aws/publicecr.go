package aws

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/DefangLabs/defang/src/pkg/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecrpublic"
)

type PublicECRAPI interface {
	GetAuthorizationToken(ctx context.Context, params *ecrpublic.GetAuthorizationTokenInput, optFns ...func(*ecrpublic.Options)) (*ecrpublic.GetAuthorizationTokenOutput, error)
}

var PublicECRClientOverride PublicECRAPI
var ecrPublicAuthToken string

func newPublicECRClientFromConfig(cfg aws.Config) PublicECRAPI {
	var svc PublicECRAPI = PublicECRClientOverride
	if svc == nil {
		cfg.Region = "us-east-1" // ECR Public is only in us-east-1
		svc = ecrpublic.NewFromConfig(cfg)
	}
	return svc
}

func (a *Aws) CheckImageExistOnPublicECR(ctx context.Context, repo, tag string) (bool, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return false, err
	}
	svc := newPublicECRClientFromConfig(cfg)
	// TODO: Not thread-safe
	if ecrPublicAuthToken == "" {
		// Fetch and cache the auth token
		authResp, err := svc.GetAuthorizationToken(ctx, &ecrpublic.GetAuthorizationTokenInput{})
		if err != nil {
			return false, err
		}
		if authResp.AuthorizationData == nil || authResp.AuthorizationData.AuthorizationToken == nil {
			return false, errors.New("no authorization data received from ECR Public")
		}
		ecrPublicAuthToken = *authResp.AuthorizationData.AuthorizationToken
	}

	manifestURL := fmt.Sprintf("https://public.ecr.aws/v2/%s/manifests/%s", repo, tag)

	// Attempt without auth first
	header := http.Header{
		"Accept":        []string{"application/vnd.docker.distribution.manifest.v2+json"},
		"Authorization": []string{"Bearer " + ecrPublicAuthToken},
	}

	resp, err := http.GetWithHeader(ctx, manifestURL, header)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return true, nil
	}
	if resp.StatusCode == 404 {
		return false, nil
	}

	body, _ := io.ReadAll(resp.Body)
	return false, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
}
