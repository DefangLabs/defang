package quota

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas/types"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

type mockAwsQuotaClient struct {
	QuotaClientAPI
}

var mockQuotas servicequotas.ListServiceQuotasOutput
var mockError error

func (m *mockAwsQuotaClient) ListServiceQuotas(ctx context.Context, params *servicequotas.ListServiceQuotasInput, optFns ...func(*servicequotas.Options)) (*servicequotas.ListServiceQuotasOutput, error) {
	return &mockQuotas, mockError
}

var quota = AwsServiceQuotas{
	quotaClient: &mockAwsQuotaClient{},
	Gpus:        8,
}

func TestGetAccountGpuQuota(t *testing.T) {
	tests := []struct {
		name      string
		quotas    servicequotas.ListServiceQuotasOutput
		err       error
		wantQuota map[string]float32
		wantErr   string
	}{
		{
			name: "has quotas",
			quotas: servicequotas.ListServiceQuotasOutput{
				Quotas: []types.ServiceQuota{
					{QuotaName: aws.String("quotaA"), Value: aws.Float64(8)},
					{QuotaName: aws.String("quotaB"), Value: aws.Float64(0)},
				},
			},
			err:       nil,
			wantQuota: map[string]float32{"quotaA": 8, "quotaB": 0},
			wantErr:   "",
		},
		{
			name: "has lowest level quota",
			quotas: servicequotas.ListServiceQuotasOutput{
				Quotas: []types.ServiceQuota{
					{QuotaName: aws.String("quotaA"), Value: aws.Float64(8)},
					{QuotaName: aws.String("quotaA"), Value: aws.Float64(4)},
				},
			},
			err:       nil,
			wantQuota: map[string]float32{"quotaA": 4},
			wantErr:   "",
		},
		{
			name:      "no permission",
			quotas:    servicequotas.ListServiceQuotasOutput{},
			err:       errors.New("no permission"),
			wantQuota: nil,
			wantErr:   "no permission",
		},
		{
			name:      "no quotas",
			quotas:    servicequotas.ListServiceQuotasOutput{},
			err:       errors.New("no permission"),
			wantQuota: nil,
			wantErr:   "no permission",
		},
	}

	var ctx = context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQuotas = tt.quotas
			mockError = tt.err
			var got, err = quota.GetAccountGpuQuota(ctx)
			if err != nil && err.Error() != tt.wantErr {
				t.Errorf("aws_quota.GetAccountGpuQuota() = %q, want %q", err, tt.wantErr)
			}

			for key, value := range got {
				if wantValue, ok := tt.wantQuota[key]; !ok || wantValue != value {
					t.Errorf("got %v, want %v for %s", value, wantValue, key)
				}
			}
		})
	}
}

func TestInitialize(t *testing.T) {
	tests := []struct {
		name     string
		quotas   servicequotas.ListServiceQuotasOutput
		err      error
		wantGpus float32
		wantErr  string
	}{
		{
			name:     "no permission",
			quotas:   servicequotas.ListServiceQuotasOutput{},
			err:      errors.New("no permission"),
			wantGpus: 0,
			wantErr:  "error getting account quota: no permission",
		},
		{
			name:     "no quotas",
			quotas:   servicequotas.ListServiceQuotasOutput{},
			err:      nil,
			wantGpus: 0,
			wantErr:  "no quotas returned",
		},
		{
			name: "has GPUs",
			quotas: servicequotas.ListServiceQuotasOutput{
				Quotas: []types.ServiceQuota{
					{QuotaName: aws.String("quotaA"), Value: aws.Float64(8)},
					{QuotaName: aws.String("quotaB"), Value: aws.Float64(2)},
				},
			},
			err:      nil,
			wantGpus: 2,
			wantErr:  "",
		},
	}

	var ctx = context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQuotas = tt.quotas
			mockError = tt.err

			if err := quota.Initialize(ctx, aws.Config{}); err != nil && err.Error() != tt.wantErr {
				t.Errorf("aws_quota.Initialize() = %q, want %q", err, tt.wantErr)
			}

			if tt.wantErr == "" && quota.Gpus != tt.wantGpus {
				t.Errorf("got %v, want %v", quota.Gpus, tt.wantGpus)
			}
		})
	}
}

func TestValidateGPUQuotas(t *testing.T) {
	tests := []struct {
		name    string
		service *composeTypes.ServiceConfig
		gpus    float32
		wantErr string
	}{
		{
			name: "only GPU",
			service: &composeTypes.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &composeTypes.DeployConfig{
					Resources: composeTypes.Resources{
						Reservations: &composeTypes.Resource{
							Devices: []composeTypes.DeviceRequest{
								{Capabilities: []string{"tpu"}},
							},
						},
					},
				},
			},
			gpus:    0,
			wantErr: "only GPU devices are supported",
		},
		{
			name: "only nvidia GPU",
			service: &composeTypes.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &composeTypes.DeployConfig{
					Resources: composeTypes.Resources{
						Reservations: &composeTypes.Resource{
							Devices: []composeTypes.DeviceRequest{
								{Capabilities: []string{"gpu"}, Driver: "amd"},
							},
						},
					},
				},
			},
			gpus:    0,
			wantErr: "only nvidia GPU devices are supported",
		},
		{
			name: "not configured for GPUs",
			service: &composeTypes.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &composeTypes.DeployConfig{
					Resources: composeTypes.Resources{
						Reservations: &composeTypes.Resource{
							Devices: []composeTypes.DeviceRequest{
								{Capabilities: []string{"gpu"}, Driver: "nvidia", Count: 1},
							},
						},
					},
				},
			},
			gpus:    0,
			wantErr: "provider not configured for GPUs",
		}, {
			name: "too many GPUs",
			service: &composeTypes.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &composeTypes.DeployConfig{
					Resources: composeTypes.Resources{
						Reservations: &composeTypes.Resource{
							Devices: []composeTypes.DeviceRequest{
								{Capabilities: []string{"gpu"}, Driver: "nvidia", Count: 100},
							},
						},
					},
				},
			},
			gpus:    8,
			wantErr: "gpu count 100 exceeds quota 8.00",
		}, {
			name: "valid service",
			service: &composeTypes.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []composeTypes.ServicePortConfig{{Target: 80, Mode: compose.Mode_INGRESS, Protocol: compose.Protocol_HTTP}},
				HealthCheck: &composeTypes.HealthCheckConfig{
					Test: []string{"CMD", "curl", "http://localhost"},
				},
				Deploy: &composeTypes.DeployConfig{
					Resources: composeTypes.Resources{
						Reservations: &composeTypes.Resource{
							NanoCPUs:    1,
							MemoryBytes: compose.MiB * 1024,
							Devices: []composeTypes.DeviceRequest{
								{
									Capabilities: []string{"gpu"},
									Driver:       "nvidia",
									Count:        1,
								},
							},
						},
					},
				},
			},
			gpus: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quota.Gpus = tt.gpus

			if err := quota.ValidateGpuQuota(context.Background(), tt.service); err != nil && err.Error() != tt.wantErr {
				t.Errorf("aws_quota.ValidateGpuQuotas() = %q, want %q", err, tt.wantErr)
			} else if err == nil && tt.wantErr != "" {
				t.Errorf("aws_quota.ValidateGpuQuotas() = nil, want %q", tt.wantErr)
			}
		})
	}
}
