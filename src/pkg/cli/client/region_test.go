package client

import (
	"testing"
)

func TestGetRegion(t *testing.T) {
	tests := []struct {
		name     string
		provider ProviderID
		envVars  map[string]string
		expected string
	}{
		{
			name:     "AWS with default",
			provider: ProviderAWS,
			envVars:  map[string]string{},
			expected: "us-west-2",
		},
		{
			name:     "AWS with AWS_REGION",
			provider: ProviderAWS,
			envVars:  map[string]string{"AWS_REGION": "us-east-1"},
			expected: "us-east-1",
		},
		{
			name:     "GCP with default",
			provider: ProviderGCP,
			envVars:  map[string]string{},
			expected: "us-central1",
		},
		{
			name:     "GCP with GCP_LOCATION (backward compatibility)",
			provider: ProviderGCP,
			envVars:  map[string]string{"GCP_LOCATION": "us-west1"},
			expected: "us-west1",
		},
		{
			name:     "GCP with GOOGLE_REGION",
			provider: ProviderGCP,
			envVars:  map[string]string{"GOOGLE_REGION": "europe-west1"},
			expected: "europe-west1",
		},
		{
			name:     "GCP with GCLOUD_REGION",
			provider: ProviderGCP,
			envVars:  map[string]string{"GCLOUD_REGION": "asia-east1"},
			expected: "asia-east1",
		},
		{
			name:     "GCP with CLOUDSDK_COMPUTE_REGION",
			provider: ProviderGCP,
			envVars:  map[string]string{"CLOUDSDK_COMPUTE_REGION": "us-east4"},
			expected: "us-east4",
		},
		{
			name:     "GCP with multiple env vars, GCP_LOCATION takes precedence",
			provider: ProviderGCP,
			envVars: map[string]string{
				"GCP_LOCATION":             "us-west1",
				"GOOGLE_REGION":            "europe-west1",
				"CLOUDSDK_COMPUTE_REGION": "asia-east1",
			},
			expected: "us-west1",
		},
		{
			name:     "GCP with multiple env vars, GOOGLE_REGION takes precedence when GCP_LOCATION is not set",
			provider: ProviderGCP,
			envVars: map[string]string{
				"GOOGLE_REGION":            "europe-west1",
				"GCLOUD_REGION":            "asia-east1",
				"CLOUDSDK_COMPUTE_REGION": "us-east4",
			},
			expected: "europe-west1",
		},
		{
			name:     "DigitalOcean with default",
			provider: ProviderDO,
			envVars:  map[string]string{},
			expected: "nyc3",
		},
		{
			name:     "DigitalOcean with REGION",
			provider: ProviderDO,
			envVars:  map[string]string{"REGION": "sfo3"},
			expected: "sfo3",
		},
		{
			name:     "Unknown provider",
			provider: ProviderID("unknown"),
			envVars:  map[string]string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test environment variables
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			got := GetRegion(tt.provider)
			if got != tt.expected {
				t.Errorf("GetRegion(%v) = %v, want %v", tt.provider, got, tt.expected)
			}
		})
	}
}
