package quota

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/compose-spec/compose-go/v2/types"
)

var (
	gpuQuotaCodes = []string{"L-7212CCBC", "L-3819A6DF"} // these are the GPU quota codes from cd 
	serviceCode   = "ec2"
)

type QuotaClientAPI interface {
	ListServiceQuotas(ctx context.Context, params *servicequotas.ListServiceQuotasInput, optFns ...func(*servicequotas.Options)) (*servicequotas.ListServiceQuotasOutput, error)
}

type AwsServiceQuotas struct {
	quotaClient QuotaClientAPI
	Gpus        float32
}

func NewAwsServiceQuotas(ctx context.Context, cfg aws.Config) *AwsServiceQuotas {
	return &AwsServiceQuotas{
		quotaClient: servicequotas.NewFromConfig(cfg),
		Gpus:        -1,
	}
}

func (q *AwsServiceQuotas) Initialize(ctx context.Context, cfg aws.Config) error {
	accountQuota, err := q.GetAccountGpuQuota(ctx)
	if err != nil {
		return fmt.Errorf("error getting account quota: %v", err)
	}

	if len(accountQuota) == 0 {
		return errors.New("no quotas returned")
	}

	cpuCount := make([]float32, 0, len(accountQuota))
	for _, v := range accountQuota {
		cpuCount = append(cpuCount, v)
	}
	sort.Slice(cpuCount, func(i, j int) bool { return cpuCount[i] < cpuCount[j] })
	q.Gpus = float32(cpuCount[0])

	return nil
}

func (q *AwsServiceQuotas) GetAccountGpuQuota(ctx context.Context) (map[string]float32, error) {
	var token *string

	result := make(map[string]float32)
	for _, quotaCode := range gpuQuotaCodes {
		for {
			quotas, err := q.quotaClient.ListServiceQuotas(ctx, &servicequotas.ListServiceQuotasInput{
				ServiceCode: aws.String(serviceCode),
				QuotaCode:   aws.String(quotaCode),
				NextToken:   token,
			})
			if err != nil {
				return nil, err
			}
			if quotas == nil {
				return nil, errors.New("no quotas returned")
			}
			for _, quota := range quotas.Quotas {
				if value, ok := result[*quota.QuotaName]; !ok || value > float32(*quota.Value) {
					result[*quota.QuotaName] = float32(*quota.Value)
				}
			}

			token = quotas.NextToken

			if token == nil {
				break
			}
		}
	}

	return result, nil
}

func (q *AwsServiceQuotas) ValidateGpuQuota(ctx context.Context, service *types.ServiceConfig) error {
	if service.Deploy.Resources.Reservations == nil {
		return nil
	}

	for _, device := range service.Deploy.Resources.Reservations.Devices {
		if len(device.Capabilities) != 1 || device.Capabilities[0] != "gpu" {
			return errors.New("only GPU devices are supported") // CodeInvalidArgument
		}

		if device.Driver != "" && device.Driver != "nvidia" {
			return errors.New("only nvidia GPU devices are supported") // CodeInvalidArgument
		}

		if q.Gpus == 0 {
			return errors.New("provider not configured for GPUs") // CodeInvalidArgument
		}

		if float32(device.Count) > q.Gpus {
			return fmt.Errorf("gpu count %v exceeds quota %.2f", device.Count, q.Gpus) // CodeInvalidArgument
		}
	}

	return nil
}
