package client

import (
	"github.com/DefangLabs/defang/src/pkg"
)

const (
	RegionDefaultAWS = "us-west-2"
	RegionDefaultDO  = "nyc3"
	RegionDefaultGCP = "us-central1" // Defaults to us-central1 for lower price
)

func GetRegion(provider ProviderID) string {
	var defaultRegion string
	switch provider {
	case ProviderAWS:
		defaultRegion = RegionDefaultAWS
	case ProviderGCP:
		defaultRegion = RegionDefaultGCP
	case ProviderDO:
		defaultRegion = RegionDefaultDO
	case ProviderDefang, ProviderAuto:
		return ""
	default:
		panic("unsupported provider")
	}

	varName := GetRegionVarName(provider)
	return pkg.Getenv(varName, defaultRegion)
}

func GetRegionVarName(provider ProviderID) string {
	switch provider {
	case ProviderAWS:
		return "AWS_REGION"
	case ProviderGCP:
		// Try standard GCP environment variables in order of precedence
		GCPRegionEnvVar, _ := pkg.GetFirstEnv(pkg.GCPRegionEnvVars...)
		if GCPRegionEnvVar == "" {
			return "GOOGLE_REGION"
		}
		return GCPRegionEnvVar
	case ProviderDO:
		return "REGION"
	case ProviderDefang, ProviderAuto:
		return ""
	default:
		panic("unsupported provider")
	}
}
