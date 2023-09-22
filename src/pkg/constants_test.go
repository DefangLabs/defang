package pkg

import "testing"

func TestQualifiedName(t *testing.T) {
	fqn := NewQualifiedName("tenantx", "servicex")
	if fqn.String() != "tenantx.servicex" {
		t.Fatal("expected fqn to be 'tenantx.servicex'")
	}
	if !fqn.IsTenant("tenantx") {
		t.Fatal("expected IsTenant to return true")
	}
	if fqn.IsTenant("tenanty") {
		t.Fatal("expected IsTenant to return false")
	}
	if fqn.Tenant() != "tenantx" {
		t.Fatal("expected GetTenantName to return 'tenantx'")
	}
	if fqn.Service() != "servicex" {
		t.Fatal("expected GetServiceName to return 'servicex'")
	}
	if QualifiedName("nodot").Service() != "" {
		t.Fatal("expected Service() to return ''")
	}
}
