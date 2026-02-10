//go:build integration

package cfn

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/stretchr/testify/assert"
)

func TestCloudFormation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test")
	}

	user := pkg.GetCurrentUser() // avoid conflict with other users in the same account
	cfn := New("crun-test-"+user, aws.Region("us-west-2"))
	if cfn == nil {
		t.Fatal("aws is nil")
	}
	cfn.RetainBucket = false // delete bucket after test
	cfn.Spot = true

	println("AWS_PROFILE=", os.Getenv("AWS_PROFILE"))
	ctx := t.Context()

	t.Run("SetUp", func(t *testing.T) {
		// Enable fancy features so we can test all conditional resources
		t.Setenv("DEFANG_NO_CACHE", "0") // force cache usage
		t.Setenv("DOCKERHUB_USERNAME", "defanglabs2")
		t.Setenv("DOCKERHUB_ACCESS_TOKEN", "defanglabs")

		err := cfn.SetUp(ctx, testContainers)
		if err != nil {
			t.Fatal(err)
		}
		if cfn.BucketName == "" {
			t.Error("bucket name is empty")
		}
	})

	t.Run("Use other region", func(t *testing.T) {
		cfnOtherRegion := New("crun-test-"+user, aws.Region("us-east-1"))
		err := cfnOtherRegion.SetUp(ctx, testContainers)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, cfn.Region, cfnOtherRegion.Region)
		assert.Equal(t, cfn.AccountID, cfnOtherRegion.AccountID)
		assert.Equal(t, cfn.BucketName, cfnOtherRegion.BucketName)
		assert.Equal(t, cfn.CIRoleARN, cfnOtherRegion.CIRoleARN)
		assert.Equal(t, cfn.ClusterName, cfnOtherRegion.ClusterName)
		assert.Equal(t, cfn.DefaultSecurityGroupID, cfnOtherRegion.DefaultSecurityGroupID)
		assert.Equal(t, cfn.LogGroupARN, cfnOtherRegion.LogGroupARN)
		assert.Equal(t, cfn.RetainBucket, cfnOtherRegion.RetainBucket)
		assert.Equal(t, cfn.SecurityGroupID, cfnOtherRegion.SecurityGroupID)
		assert.Equal(t, cfn.Spot, cfnOtherRegion.Spot)
		assert.Equal(t, cfn.SubNetID, cfnOtherRegion.SubNetID)
		assert.Equal(t, cfn.TaskDefARN, cfnOtherRegion.TaskDefARN)
		assert.Equal(t, cfn.VpcID, cfnOtherRegion.VpcID)
	})

	var taskid clouds.TaskID
	t.Run("Run", func(t *testing.T) {
		var err error
		taskid, err = cfn.Run(ctx, nil, "echo", "hello")
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
		t.Cleanup(cancel)
		err := cfn.Tail(ctx, taskid)
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
	})

	t.Run("Stop", func(t *testing.T) {
		if taskid == nil {
			t.Skip("task id is empty")
		}
		err := cfn.Stop(ctx, taskid)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Teardown", func(t *testing.T) {
		// This will fail if the task is still running
		err := cfn.TearDown(ctx)
		if err != nil {
			t.Fatal(err)
		}
	})
}
