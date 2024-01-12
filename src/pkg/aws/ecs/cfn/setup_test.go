//go:build integration

package cfn

import (
	"context"
	"testing"

	"github.com/defang-io/defang/src/pkg/aws/region"
)

func TestNew(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	aws := New("crun-test", region.Region("us-west-2")) // TODO: customize name
	if aws == nil {
		t.Fatal("aws is nil")
	}

	ctx := context.TODO()

	t.Run("SetUp", func(t *testing.T) {
		err := aws.SetUp(ctx, "docker.io/library/alpine:latest", 512_000_000, "linux/amd64")
		if err != nil {
			t.Fatal(err)
		}
		if aws.BucketName == "" {
			t.Error("bucket name is empty")
		}
	})

	t.Run("Teardown", func(t *testing.T) {
		err := aws.TearDown(ctx)
		if err != nil {
			t.Fatal(err)
		}
	})
}
