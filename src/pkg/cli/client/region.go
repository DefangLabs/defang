package client

import "github.com/DefangLabs/defang/src/pkg"

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
		return "GCP_LOCATION"
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
