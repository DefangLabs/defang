package client

import (
	"fmt"
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ProviderID string

const (
	ProviderAuto   ProviderID = "auto"
	ProviderDefang ProviderID = "defang"
	ProviderAWS    ProviderID = "aws"
	ProviderDO     ProviderID = "digitalocean"
	ProviderGCP    ProviderID = "gcp"
	ProviderAzure  ProviderID = "azure"
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
	case ProviderDefang:
		return "Defang Playground"
	case ProviderAWS:
		return "AWS"
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
	case ProviderDefang:
		return defangv1.Provider_DEFANG
	case ProviderAWS:
		return defangv1.Provider_AWS
	case ProviderDO:
		return defangv1.Provider_DIGITALOCEAN
	case ProviderGCP:
		return defangv1.Provider_GCP
	default:
		return defangv1.Provider_PROVIDER_UNSPECIFIED
	}
}

func (p *ProviderID) Set(str string) error {
	str = strings.ToLower(str)
	for _, provider := range allProviders {
		if provider.String() == str {
			*p = provider
			return nil
		}
	}

	return fmt.Errorf("provider not one of %v", allProviders)
}

func (p *ProviderID) SetValue(val defangv1.Provider) {
	switch val {
	case defangv1.Provider_DEFANG:
		*p = ProviderDefang
	case defangv1.Provider_AWS:
		*p = ProviderAWS
	case defangv1.Provider_DIGITALOCEAN:
		*p = ProviderDO
	case defangv1.Provider_GCP:
		*p = ProviderGCP
	case defangv1.Provider_AZURE:
		*p = ProviderAzure
	default:
		*p = ProviderAuto
	}
}

func (p ProviderID) Type() string {
	return "provider"
}
