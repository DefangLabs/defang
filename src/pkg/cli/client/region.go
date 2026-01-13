package client

import (
	"github.com/DefangLabs/defang/src/pkg"
)

func GetRegion(provider ProviderID) string {
	switch provider {
	case ProviderAWS:
		return pkg.Getenv("AWS_REGION", "us-west-2") // Default region for AWS
	case ProviderGCP:
		// Try standard GCP environment variables in order of precedence
		// Keeping GCP_LOCATION first for backward compatibility
		_, region := pkg.GetFirstEnv(pkg.GCPRegionEnvVars...)
		if region == "" {
			return "us-central1" // Default region for GCP
		}
		return region
	case ProviderDO:
		return pkg.Getenv("REGION", "nyc3") // Default region for DigitalOcean
	default:
		return "" // No default region for unsupported providers or playground
	}
}
