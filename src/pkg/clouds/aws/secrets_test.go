//go:build integration

package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/google/uuid"
)

func isErrCodeNotFound(err error) bool {
	var e *types.ParameterNotFound
	return errors.As(err, &e)
}

func TestPutConfig(t *testing.T) {
	a := Aws{Region: Region(pkg.Getenv("AWS_REGION", "us-west-2"))}

	ctx := context.Background()
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Create an instance of AWS SSM
	svc := ssm.NewFromConfig(cfg)

	// Create random secret name and value
	rootPath = "/"
	name := uuid.NewString()
	value := uuid.NewString()
	secretId := rootPath + name
	exist, err := a.IsValidConfig(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if exist {
		t.Fatal("secret should not exist")
	}

	err = a.PutConfig(ctx, name, value, true)
	if err != nil {
		t.Fatal(err)
	}

	// Cleanup after test
	defer a.DeleteConfigs(ctx, name)

	configValues, err := a.GetConfigs(ctx, []string{name}, rootPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(configValues.Configs) == 1 {
		t.Fatalf("expected 1 config, got %d", len(configValues.Configs))
	}

	if value != configValues.Configs[0].Value {
		t.Fatalf("expected %s, got %s", value, configValues.Configs[0].Value)
	}

	exist, err = a.IsValidConfigName(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if !exist {
		t.Fatal("secret should exist")
	}

	// Check that the secret is in the list
	ls, err := a.ListConfigs(ctx)
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
	err = a.PutConfig(ctx, name, "new value", false)
	if err != nil {
		t.Fatal(err)
	}

	// Overwrite secret with empty; this should delete the secret
	err = a.DeleteConfigs(ctx, name)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the secret is deleted
	_, err = a.GetConfigs(ctx, rootPath, name)
	if !isErrCodeNotFound(err) {
		t.Fatalf("expected ErrCodeParameterNotFound, got %v", err)
	}

	// Delete the secret again; this should return NotFound
	err = a.DeleteConfigs(ctx, name)
	if !isErrCodeNotFound(err) {
		t.Fatalf("expected ErrCodeParameterNotFound, got %v", err)
	}

	// Check that the secret is not in the list; delete is lazy so retry for up to 5 seconds
	for i := 0; i < 5; i++ {
		ls, err = a.ListConfigs(ctx)
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
