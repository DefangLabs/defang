package client

import (
	"fmt"
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ProviderID string

const (
	ProviderAuto   ProviderID = "auto"
	ProviderAWS    ProviderID = "aws"
	ProviderAzure  ProviderID = "azure"
	ProviderDefang ProviderID = "defang"
	ProviderDO     ProviderID = "digitalocean"
	ProviderGCP    ProviderID = "gcp"
)

var allProviders = []ProviderID{
	ProviderAuto,
	ProviderDefang,
	ProviderAWS,
	ProviderDO,
	ProviderGCP,
	ProviderAzure,
}

func AllProviders() []ProviderID {
	return allProviders[1:] // skip "auto"
}

func (p ProviderID) String() string {
	return string(p)
}

func (p ProviderID) Name() string {
	switch p {
	case ProviderAuto:
		return "Auto"
	case ProviderAWS:
		return "AWS"
	case ProviderAzure:
		return "Azure"
	case ProviderDefang:
		return "Defang Playground"
	case ProviderDO:
		return "DigitalOcean"
	case ProviderGCP:
		return "Google Cloud Platform"
	default:
		return p.String()
	}
}

func (p ProviderID) Value() defangv1.Provider {
	switch p {
	case ProviderAWS:
		return defangv1.Provider_AWS
	case ProviderAzure:
		return defangv1.Provider_AZURE
	case ProviderDefang:
		return defangv1.Provider_DEFANG
	case ProviderDO:
		return defangv1.Provider_DIGITALOCEAN
	case ProviderGCP:
		return defangv1.Provider_GCP
	default:
		return defangv1.Provider_PROVIDER_UNSPECIFIED
	}
}

func (p *ProviderID) Set(str string) error {
	for _, provider := range allProviders {
		if strings.EqualFold(str, provider.String()) || strings.EqualFold(str, provider.Name()) {
			*p = provider
			return nil
		}
	}

	return fmt.Errorf("invalid provider: %q, not one of %v", str, allProviders)
}

func (p *ProviderID) SetValue(val defangv1.Provider) {
	switch val {
	case defangv1.Provider_AWS:
		*p = ProviderAWS
	case defangv1.Provider_AZURE:
		*p = ProviderAzure
	case defangv1.Provider_DEFANG:
		*p = ProviderDefang
	case defangv1.Provider_DIGITALOCEAN:
		*p = ProviderDO
	case defangv1.Provider_GCP:
		*p = ProviderGCP
	default:
		*p = ProviderAuto
	}
}

func (p ProviderID) Type() string {
	return "provider"
}
