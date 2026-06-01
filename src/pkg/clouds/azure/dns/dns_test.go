package dns

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

func TestNameServers(t *testing.T) {
	tests := []struct {
		name string
		zone armdns.Zone
		want []string
	}{
		{name: "nil properties", zone: armdns.Zone{}, want: []string{}},
		{
			name: "empty list",
			zone: armdns.Zone{Properties: &armdns.ZoneProperties{}},
			want: []string{},
		},
		{
			name: "skips nil entries",
			zone: armdns.Zone{Properties: &armdns.ZoneProperties{
				NameServers: []*string{to.Ptr("ns1-01.azure-dns.com."), nil, to.Ptr("ns2-01.azure-dns.net.")},
			}},
			want: []string{"ns1-01.azure-dns.com.", "ns2-01.azure-dns.net."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nameServers(tt.zone)
			if len(got) != len(tt.want) {
				t.Fatalf("nameServers() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("nameServers()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestEnsureZoneExistsCredError(t *testing.T) {
	// A failing credential must surface from EnsureZoneExists rather than
	// silently returning empty name servers.
	orig := azure.NewCredsFunc
	azure.NewCredsFunc = func(azure.Azure) (azcore.TokenCredential, error) {
		return nil, errors.New("no creds")
	}
	t.Cleanup(func() { azure.NewCredsFunc = orig })

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	d := New("rg", azure.Azure{SubscriptionID: "sub"})
	if _, err := d.EnsureZoneExists(ctx, "example.com"); err == nil {
		t.Error("EnsureZoneExists should surface credential error")
	}
}
