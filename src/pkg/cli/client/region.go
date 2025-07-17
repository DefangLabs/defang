package client

import "github.com/DefangLabs/defang/src/pkg"

func GetRegion(provider ProviderID) string {
	switch provider {
	case ProviderAWS:
		return pkg.Getenv("AWS_REGION", "us-west-2") // Default region for AWS
	case ProviderGCP:
		return pkg.Getenv("CLOUDSDK_COMPUTE_REGION", "us-central1") // Default region for GCP
	case ProviderDO:
		return pkg.Getenv("DO_REGION", "nyc3") // Default region for DigitalOcean
	default:
		return "" // No default region for unsupported providers
	}
}
