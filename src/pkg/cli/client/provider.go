package client

import (
	"fmt"
	"strings"
)

type Provider string

const (
	ProviderAuto   Provider = "auto"
	ProviderDefang Provider = "defang"
	ProviderAWS    Provider = "aws"
	ProviderDO     Provider = "do"
	// ProviderAzure  Provider = "azure"
	// ProviderGCP    Provider = "gcp"
)

var allProviders = []Provider{
	ProviderAuto,
	ProviderDefang,
	ProviderAWS,
	ProviderDO,
	// ProviderAzure,
	// ProviderGCP,
}

func (p Provider) String() string {
	return string(p)
}

func (p *Provider) Set(str string) error {
	str = strings.ToLower(str)
	for _, provider := range allProviders {
		if provider.String() == str {
			*p = provider
			return nil
		}
	}

	return fmt.Errorf("available providers are: %v", allProviders)
}

func (p Provider) Type() string {
	return "provider"
}
