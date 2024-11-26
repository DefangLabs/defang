package aws

import (
	"context"
	"errors"
	"slices"

	"github.com/DefangLabs/defang/src/pkg/cli/permissions"
	"github.com/DefangLabs/defang/src/pkg/store"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

var (
	gpuQuotaCodes = []string{"L-7212CCBC", "L-3819A6DF"} // these are the GPU quota codes from cd
	serviceCode   = "ec2"
)

type QuotaClientAPI interface {
	ListServiceQuotas(ctx context.Context, params *servicequotas.ListServiceQuotasInput, optFns ...func(*servicequotas.Options)) (*servicequotas.ListServiceQuotasOutput, error)
}

var quotaClient QuotaClientAPI

var ErrAWSNoConnection = errors.New("no connect to AWS service quotas")
var ErrGPUQuotaZero = errors.New("no GPUs enabled. To resolve see https://docs.defang.io/docs/tutorials/deploy-with-gpu")
var ErrNoQuotasReceived = errors.New("no service quotas received")

func NewServiceQuotasClient(ctx context.Context, cfg aws.Config) *servicequotas.Client {
	return servicequotas.NewFromConfig(cfg)
}

func hasGPUQuota(ctx context.Context) (bool, error) {
	if quotaClient == nil {
		return false, ErrAWSNoConnection
	}

	var token *string
	for _, quotaCode := range gpuQuotaCodes {
		for {
			quotas, err := quotaClient.ListServiceQuotas(ctx, &servicequotas.ListServiceQuotasInput{
				ServiceCode: aws.String(serviceCode),
				QuotaCode:   aws.String(quotaCode),
				NextToken:   token,
			})
			if err != nil {
				return false, err
			}
			if len(quotas.Quotas) == 0 {
				return false, ErrNoQuotasReceived
			}

			// the quota.Value is actually the number of CPUs, but since we only
			// alllocate GPU enabled instances, as soon as we know there
			// is a non-zero CPU instance we know that there is at least one GPU
			for _, quota := range quotas.Quotas {
				if *(quota.Value) > 0.0 {
					return true, nil
				}
			}

			token = quotas.NextToken
			if token == nil {
				break
			}
		}
	}

	// if we've reached this point, no GPU quota was found or all quotas were zero
	return false, nil
}

func ValidateGPUResources(ctx context.Context, project *composeTypes.Project, mode defangv1.DeploymentMode) error {
	// return after checking if there are actually non-zero GPUs requested
	hasGPUs, quotaErr := hasGPUQuota(ctx)

	for _, service := range project.Services {
		if service.Deploy != nil &&
			service.Deploy.Resources.Reservations != nil {
			for _, device := range service.Deploy.Resources.Reservations.Devices {
				if slices.Contains(device.Capabilities, "gpu") {
					if device.Count == 0 {
						continue
					}

					// if there was an error getting the quota
					if quotaErr != nil {
						return quotaErr
					}

					if err := permissions.HasPermission(store.UserWhoAmI.Tier, "use-gpu", "gpu", float64(device.Count), "not enough GPUs permitted at current subscription tier"); err != nil {
						return err
					}

					if err := permissions.HasPermission(store.UserWhoAmI.Tier, "use-gpu", "mode", float64(mode.Number()), "not enough GPUs permitted at current subscription tier"); err != nil {
						return err
					}

					if !hasGPUs {
						return ErrGPUQuotaZero
					}

					break
				}
			}
		}
	}

	return nil
}
