package aws

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
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
var ErrGPUQuotaZero = errors.New("no GPUs enabled. To resolve see https://s.defang.io/deploy-gpu")
var ErrNoQuotasReceived = errors.New("no service quotas received")

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

func validateGPUResources(ctx context.Context, project *composeTypes.Project) error {
	// return after checking if there are actually non-zero GPUs requested
	gpusAvailable, quotaErr := hasGPUQuota(ctx)

	gpuCount := compose.GetNumOfGPUs(project.Services)
	if gpuCount == 0 {
		return nil
	}

	// if there was an error getting the quota
	if quotaErr != nil {
		return quotaErr
	}

	if !gpusAvailable {
		return ErrGPUQuotaZero
	}

	return nil
}
