//go:build integration

package cfn

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/region"
)

func TestCloudFormation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test")
	}

	user := pkg.GetCurrentUser() // avoid conflict with other users in the same account
	aws := New("crun-test-"+user, region.Region("us-west-2"))
	if aws == nil {
		t.Fatal("aws is nil")
	}
	aws.RetainBucket = false // delete bucket after test
	aws.Spot = true

	ctx := t.Context()

	t.Run("SetUp", func(t *testing.T) {
		// Enable fancy features so we can test all conditional resources
		t.Setenv("DEFANG_NO_CACHE", "0") // force cache usage
		t.Setenv("DOCKERHUB_USERNAME", "defanglabs2")
		t.Setenv("DOCKERHUB_ACCESS_TOKEN", "defanglabs")

		_, err := aws.SetUp(ctx, testContainers)
		if err != nil {
			t.Fatal(err)
		}
		if aws.BucketName == "" {
			t.Error("bucket name is empty")
		}
	})

	var taskid clouds.TaskID
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
