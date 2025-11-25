package aws

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type SecretManagerAPI interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

var SecretManagerClientOverride SecretManagerAPI

func newSecretManagerFromConfig(cfg aws.Config) SecretManagerAPI {
	var svc SecretManagerAPI = SecretManagerClientOverride

	if svc == nil {
		svc = secretsmanager.NewFromConfig(cfg)
	}

	return svc
}

func (a *Aws) GetValueFromSecretManager(ctx context.Context, arn string) (string, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return "", err
	}

	svc := newSecretManagerFromConfig(cfg)

	res, err := svc.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &arn,
	})

	if err != nil {
		return "", err
	}

	if res.SecretString == nil {
		return "", errors.New("secret has no value")
	}
	return *res.SecretString, nil
}
