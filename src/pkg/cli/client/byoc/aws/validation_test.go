package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	aws "github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/cfn"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	quotaTypes "github.com/aws/aws-sdk-go-v2/service/servicequotas/types"
	"github.com/aws/smithy-go"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

var errAWSOperation *smithy.OperationError

type MockQuotaClientApi struct {
	QuotaClientAPI
	output *servicequotas.ListServiceQuotasOutput
	err    error
}

func (q *MockQuotaClientApi) ListServiceQuotas(ctx context.Context, params *servicequotas.ListServiceQuotasInput, optFns ...func(*servicequotas.Options)) (*servicequotas.ListServiceQuotasOutput, error) {
	return q.output, q.err
}

var ctx = context.Background()
var mockQuotaClient = &MockQuotaClientApi{}

func TestValidateGPUResources(t *testing.T) {
	t.Run("No service quuota received", func(t *testing.T) {
		testService := composeTypes.ServiceConfig{
			Deploy: &composeTypes.DeployConfig{
				Resources: composeTypes.Resources{
					Reservations: &composeTypes.Resource{
						Devices: []composeTypes.DeviceRequest{
							{Capabilities: []string{"gpu"}, Count: 0},
						},
					},
				},
			},
		}
		project := composeTypes.Project{
			Services: map[string]composeTypes.ServiceConfig{
				"test": testService,
			},
		}

		quotaClient = mockQuotaClient
		mockQuotaClient.output = nil
		mockQuotaClient.err = ErrNoQuotasReceived

		err := ValidateGPUResources(ctx, &project)
		if err != nil && errors.Is(err, ErrNoQuotasReceived) {
			t.Fatalf("ValidateGPUResources() failed: Unexpected errors %v", err)
		}
	})

	t.Run("no errors when gpu is set to 0", func(t *testing.T) {
		testService := composeTypes.ServiceConfig{
			Deploy: &composeTypes.DeployConfig{
				Resources: composeTypes.Resources{
					Reservations: &composeTypes.Resource{
						Devices: []composeTypes.DeviceRequest{
							{Capabilities: []string{"gpu"}, Count: 0},
						},
					},
				},
			},
		}
		project := composeTypes.Project{
			Services: map[string]composeTypes.ServiceConfig{
				"test": testService,
			},
		}

		quotaClient = nil
		mockQuotaClient.err = nil
		err := ValidateGPUResources(ctx, &project)
		if err != nil {
			t.Fatalf("ValidateGPUResources() failed: expected no errors but got %v", err)
		}
	})

	t.Run("no gpu quota but requesting one", func(t *testing.T) {
		testService := composeTypes.ServiceConfig{
			Deploy: &composeTypes.DeployConfig{
				Resources: composeTypes.Resources{
					Reservations: &composeTypes.Resource{
						Devices: []composeTypes.DeviceRequest{
							{Capabilities: []string{"gpu"}, Count: 24},
						},
					},
				},
			},
		}
		project := composeTypes.Project{
			Services: map[string]composeTypes.ServiceConfig{
				"test": testService,
			},
		}

		quotaClient = mockQuotaClient
		mockQuotaClient.err = nil
		mockQuotaClient.output = &servicequotas.ListServiceQuotasOutput{
			Quotas: []quotaTypes.ServiceQuota{
				{
					QuotaCode: awssdk.String("AWS_ECS_GPU_LIMIT"),
					Value:     awssdk.Float64(0),
				},
			},
		}
		err := ValidateGPUResources(ctx, &project)
		if err != nil && !errors.Is(err, ErrGPUQuotaZero) {
			t.Fatalf("ValidateGPUResources() failed: Unexpected err %v", err)
		}
	})

	t.Run("gpu quota exists but requesting one", func(t *testing.T) {
		testService := composeTypes.ServiceConfig{
			Deploy: &composeTypes.DeployConfig{
				Resources: composeTypes.Resources{
					Reservations: &composeTypes.Resource{
						Devices: []composeTypes.DeviceRequest{
							{Capabilities: []string{"gpu"}, Count: 24},
						},
					},
				},
			},
		}
		project := composeTypes.Project{
			Services: map[string]composeTypes.ServiceConfig{
				"test": testService,
			},
		}

		quotaClient = mockQuotaClient
		mockQuotaClient.err = nil
		mockQuotaClient.output = &servicequotas.ListServiceQuotasOutput{
			Quotas: []quotaTypes.ServiceQuota{
				{
					QuotaCode: awssdk.String("AWS_ECS_GPU_LIMIT"),
					Value:     awssdk.Float64(1),
				},
			},
		}
		err := ValidateGPUResources(ctx, &project)
		if err != nil {
			t.Fatalf("ValidateGPUResources() failed: Unexpected err %v", err)
		}
	})

	t.Run("unable to get AWS gpu quota", func(t *testing.T) {
		testService := composeTypes.ServiceConfig{
			Deploy: &composeTypes.DeployConfig{
				Resources: composeTypes.Resources{
					Reservations: &composeTypes.Resource{
						Devices: []composeTypes.DeviceRequest{
							{Capabilities: []string{"gpu"}, Count: 24},
						},
					},
				},
			},
		}

		project := composeTypes.Project{
			Services: map[string]composeTypes.ServiceConfig{
				"test": testService,
			},
		}

		quotaClient = nil
		err := ValidateGPUResources(ctx, &project)
		if err != nil && !errors.Is(err, ErrAWSNoConnection) {
			t.Fatalf("ValidateGPUResources() failed: Unexpected err %v", err)
		}
	})
}

func TestDeployValidateGPUResources(t *testing.T) {
	quotaClient = nil

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

		quotaClient = nil
		_, err := b.Deploy(ctx, &testDeploy)
		if err != nil && (errors.Is(err, ErrGPUQuotaZero)) {
			t.Fatalf("Deploy() failed: expected no GPU errors but got %v", err)
		}
	})

	t.Run("error on too many gpu", func(t *testing.T) {
		t.Skip("This is making actual AWS calls, need to mock out more")
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
		if err != nil && !errors.Is(err, ErrGPUQuotaZero) && !errors.As(err, &errAWSOperation) {
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
		if err != nil && errors.Is(err, ErrGPUQuotaZero) {
			t.Fatal("Deploy() failed: Unexpected ErrGPUQuotaZero")
		}
	})
}
