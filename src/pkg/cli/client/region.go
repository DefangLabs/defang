package client

import (
	"github.com/DefangLabs/defang/src/pkg"
)

func GetRegion(provider ProviderID) string {
	varName := GetRegionVarName(provider)
	var defaultRegion string
	switch provider {
	case ProviderAWS:
		defaultRegion = "us-west-2"
	case ProviderGCP:
		defaultRegion = "us-central1"
	case ProviderDO:
		defaultRegion = "nyc3"
	case ProviderDefang:
		return ""
	case ProviderAuto:
		return ""
	default:
		panic("unsupported provider")
	}

	return pkg.Getenv(varName, defaultRegion)
}

func GetRegionVarName(provider ProviderID) string {
	switch provider {
	case ProviderAWS:
		return "AWS_REGION"
	case ProviderGCP:
		// Try standard GCP environment variables in order of precedence
		// Keeping GCP_LOCATION first for backward compatibility
		_, GCPRegion := pkg.GetFirstEnv(pkg.GCPRegionEnvVars...)
		return GCPRegion
	case ProviderDO:
		return "REGION"
	case ProviderDefang:
		return ""
	case ProviderAuto:
		return ""
	default:
		panic("unsupported provider")
	}
}
