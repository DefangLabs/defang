package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/cfn"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestValidateGPUResources(t *testing.T) {
	ctx := context.Background()

	//like calling NewByocProvider(), but without needing real AccountInfo data
	b := &ByocAws{
		driver: cfn.New(byoc.CdTaskPrefix, aws.Region("")), // default region
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(context.Background(), "tenant1", b)
	b.ByocBaseClient.SetupDone = true

	t.Run("no errors", func(t *testing.T) {
		testDeploy := defangv1.DeployRequest{
			Compose: []byte(
				`name: project
services:
  app:
    image: defanglabs/app:latest
    deploy:
      resources:
        reservations:
          devices:
            - capabilities: [gpu]
              count: 1
`),
		}

		_, err := b.Deploy(ctx, &testDeploy)
		if err != nil && (errors.Is(err, ErrGPUQuotaExceeded) || errors.Is(err, ErrZeroGPUsRequested)) {
			t.Fatalf("Deploy() failed: expected no GPU errors but got %v", err)
		}
	})

	t.Run("error on too many gpu", func(t *testing.T) {
		testDeploy := defangv1.DeployRequest{
			Compose: []byte(
				`name: project
services:
  app:
    image: defanglabs/app:latest
    deploy:
      resources:
        reservations:
          devices:
            - capabilities: [gpu]
              count: 24
`),
		}

		_, err := b.Deploy(ctx, &testDeploy)
		if err != nil && !errors.Is(err, ErrGPUQuotaExceeded) {
			t.Fatalf("Deploy() failed: Unexpected error %v", err)
		}
	})

	t.Run("no error on non-gpu resource", func(t *testing.T) {
		testDeploy := defangv1.DeployRequest{
			Compose: []byte(
				`name: project
services:
  app:
    image: defanglabs/app:latest
    deploy:
      resources:
        reservations:
          devices:
            - capabilities: [not-gpu]
              count: 24
`),
		}

		_, err := b.Deploy(ctx, &testDeploy)
		if err != nil && errors.Is(err, ErrGPUQuotaExceeded) {
			t.Fatal("Deploy() failed: Unexpected ErrGPUQuotaExceeded")
		}
	})

	t.Run("error on no gpu", func(t *testing.T) {
		testDeploy := defangv1.DeployRequest{
			Compose: []byte(
				`name: project
services:
  app:
    image: defanglabs/app:latest
    deploy:
      resources:
        reservations:
          devices:
            - capabilities: [gpu]
              count: 0
`),
		}

		_, err := b.Deploy(ctx, &testDeploy)
		if err != nil && !errors.Is(err, ErrZeroGPUsRequested) {
			t.Fatalf("Deploy() failed: Unexpected error %v", err)
		}
	})
}
