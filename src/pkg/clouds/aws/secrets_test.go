//go:build integration

package aws

import (
	"context"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/google/uuid"
)

func TestPutSecret(t *testing.T) {
	a := Aws{Region: Region(pkg.Getenv("AWS_REGION", "us-west-2"))}

	ctx := context.Background()
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Create an instance of AWS SSM
	svc := ssm.NewFromConfig(cfg)

	// Create random secret name and value
	name := uuid.NewString()
	value := uuid.NewString()
	secretId := name // caller should have added any prefix

	exist, err := a.IsValidSecret(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if exist {
		t.Fatal("secret should not exist")
	}

	err = a.PutSecret(ctx, name, value)
	if err != nil {
		t.Fatal(err)
	}
	// Cleanup after test
	defer svc.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: &secretId,
	})

	gsv, err := svc.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &secretId,
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if value != *gsv.Parameter.Value {
		t.Fatalf("expected %s, got: %s", value, *gsv.Parameter.Value)
	}

	exist, err = a.IsValidSecret(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if !exist {
		t.Fatal("secret should exist")
	}

	// Check that the secret is in the list
	ls, err := a.ListSecrets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range ls {
		if s == name {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s in list, got %v", name, ls)
	}

	// Overwrite secret with a new value
	err = a.PutSecret(ctx, name, "new value")
	if err != nil {
		t.Fatal(err)
	}

	// Overwrite secret with empty; this should delete the secret
	err = a.DeleteSecrets(ctx, name)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the secret is deleted
	_, err = svc.GetParameter(ctx, &ssm.GetParameterInput{
		Name: &secretId,
	})
	if !IsParameterNotFoundError(err) {
		t.Fatalf("expected ErrCodeParameterNotFound, got %v", err)
	}

	// Delete the secret again; this should return NotFound
	err = a.DeleteSecrets(ctx, name)
	if !IsParameterNotFoundError(err) {
		t.Fatalf("expected ErrCodeParameterNotFound, got %v", err)
	}

	// Check that the secret is not in the list; delete is lazy so retry for up to 5 seconds
	for i := 0; i < 5; i++ {
		ls, err = a.ListSecrets(ctx)
		if err != nil {
			t.Fatal(err)
		}
		found = false
		for _, s := range ls {
			if s == name {
				found = true
				break
			}
		}
		if !found {
			break
		}
		time.Sleep(time.Second)
	}
	if found {
		t.Fatalf("expected %s not in list, got %v", name, ls)
	}
}
