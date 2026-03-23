//go:build integration

package cfn

import (
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
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

	ctx := t.Context()

	t.Run("SetUp", func(t *testing.T) {
		// Enable fancy features so we can test all conditional resources
		t.Setenv("DEFANG_NO_CACHE", "0") // force cache usage
		t.Setenv("DOCKERHUB_USERNAME", "defanglabs2")
		t.Setenv("DOCKERHUB_ACCESS_TOKEN", "defanglabs")

		_, err := aws.SetUp(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		if aws.BucketName == "" {
			t.Error("bucket name is empty")
		}
		if aws.ProjectName == "" {
			t.Error("project name is empty")
		}
	})

	t.Run("Run", func(t *testing.T) {
		taskid, err := aws.Run(ctx, "/app", "aws/codebuild/amazonlinux2-x86_64-standard:5.0", nil, "echo", "hello")
		if err != nil {
			t.Fatal(err)
		}
		if taskid == nil || *taskid == "" {
			t.Error("build id is empty")
		}

		t.Run("Stop", func(t *testing.T) {
			err := aws.Stop(ctx, taskid)
			if err != nil {
				t.Fatal(err)
			}
		})
	})

	t.Run("Teardown", func(t *testing.T) {
		err := aws.TearDown(ctx)
		if err != nil {
			t.Fatal(err)
		}
	})
}
