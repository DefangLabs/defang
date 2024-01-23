package client

import (
	"fmt"
	"strings"
)

type Provider string

const (
	ProviderDefang Provider = "defang"
	ProviderAWS    Provider = "aws"
	// ProviderAzure  Provider = "azure"
	// ProviderGCP    Provider = "gcp"
)

var providerMap = map[string]Provider{
	"defang": ProviderDefang,
	"aws":    ProviderAWS,
	// "azure":  ProviderAzure,
	// "gcp":    ProviderGCP,
}

func (p Provider) String() string {
	return string(p)
}

func (p *Provider) Set(str string) error {
	if provider, ok := providerMap[str]; ok {
		p = &provider
		return nil
	}

	availableProviders := make([]string, 0, len(providerMap))
	for provider := range providerMap {
		availableProviders = append(availableProviders, provider)
	}
	return fmt.Errorf("invalid provider '%v', available providers are: %s", str, strings.Join(availableProviders, ", "))
}

func (p Provider) Type() string {
	return "Provider"
}
