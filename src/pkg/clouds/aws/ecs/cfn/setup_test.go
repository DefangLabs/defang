//go:build integration

package cfn

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/types"
)

func TestCloudFormation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test")
	}

	retainBucket = false // delete bucket after test

	user := pkg.GetCurrentUser() // avoid conflict with other users in the same account
	aws := New("crun-test-"+user, aws.RegionUSWest2)
	if aws == nil {
		t.Fatal("aws is nil")
	}

	ctx := context.Background()

	t.Run("SetUp", func(t *testing.T) {
		containers := []types.Container{{
			Image:    "public.ecr.aws/docker/library/alpine:latest",
			Memory:   512_000_000,
			Platform: "linux/amd64",
		}}
		err := aws.SetUp(ctx, containers)
		if err != nil {
			t.Fatal(err)
		}
		if aws.BucketName == "" {
			t.Error("bucket name is empty")
		}
	})

	var taskid types.TaskID
	t.Run("Run", func(t *testing.T) {
		var err error
		taskid, err = aws.Run(ctx, nil, "echo", "hello")
		if err != nil {
			t.Fatal(err)
		}
		if taskid == nil || *taskid == "" {
			t.Error("task id is empty")
		}
	})

	t.Run("Tail", func(t *testing.T) {
		if taskid == nil {
			t.Skip("task id is empty")
		}
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		err := aws.Tail(ctx, taskid)
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
	})

	t.Run("Stop", func(t *testing.T) {
		if taskid == nil {
			t.Skip("task id is empty")
		}
		err := aws.Stop(ctx, taskid)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Teardown", func(t *testing.T) {
		// This will fail if the task is still running
		err := aws.TearDown(ctx)
		if err != nil {
			t.Fatal(err)
		}
	})
}
