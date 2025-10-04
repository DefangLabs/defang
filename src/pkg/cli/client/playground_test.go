package client

import (
	"testing"
)

func TestServiceDNS(t *testing.T) {
	p := PlaygroundProvider{FabricClient: GrpcClient{TenantName: "proj1"}}

	const expected = "proj1-service1"
	if got := p.ServicePrivateDNS("service1"); got != expected {
		t.Errorf("ServicePrivateDNS() = %v, want %v", got, expected)
	}
}
