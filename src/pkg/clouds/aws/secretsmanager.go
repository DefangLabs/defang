package aws

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/smithy-go/ptr"
)

type SecretManagerAPI interface {
	CreateSecret(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	UpdateSecret(ctx context.Context, params *secretsmanager.UpdateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UpdateSecretOutput, error)
}

func PutSecretManagerSecret(ctx context.Context, name, value string, svc SecretManagerAPI) (string, error) {
	secretId := ptr.String(name)

	out, err := svc.UpdateSecret(ctx, &secretsmanager.UpdateSecretInput{
		SecretId:     secretId,
		SecretString: ptr.String(value),
	})

	if err != nil {
		var nfe *types.ResourceNotFoundException
		if !errors.As(err, &nfe) {
			return "", err
		}

		_, err = svc.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
			Name:         secretId,
			SecretString: ptr.String(value),
		})
		var ee *types.ResourceExistsException
		if err != nil && !errors.As(err, &ee) { // Ignore resource exist, maybe created in race condition
			return "", err
		}

		out, err = svc.UpdateSecret(ctx, &secretsmanager.UpdateSecretInput{
			SecretId:     secretId,
			SecretString: ptr.String(value),
		})
		if err != nil {
			return "", err
		}
	}

	if out.VersionId == nil || out.ARN == nil {
		return "", errors.New("PutSecretManagerSecret: missing ARN or VersionId in response")
	}

	versionedArn := *out.ARN + "::" + *out.VersionId
	return versionedArn, nil
}
