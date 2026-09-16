package loadbalancer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	cloudazure "github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

type fakeCredential struct{}

func (fakeCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func TestAllBackendsHealthy(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"all healthy", `{"value":[{"timeseries":[{"data":[{"average":100,"timeStamp":"2026-09-16T18:15:00Z"}]},{"data":[{"average":100,"timeStamp":"2026-09-16T18:15:00Z"}]}]}]}`, true},
		{"one unhealthy", `{"value":[{"timeseries":[{"data":[{"average":100,"timeStamp":"2026-09-16T18:15:00Z"}]},{"data":[{"average":0,"timeStamp":"2026-09-16T18:15:00Z"}]}]}]}`, false},
		{"stale healthy sample", `{"value":[{"timeseries":[{"data":[{"average":100,"timeStamp":"2026-09-16T18:13:00Z"}]}]}]}`, false},
		{"no samples", `{"value":[{"timeseries":[]}]}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("$filter"); got != "BackendIPAddress eq '*'" {
					t.Errorf("$filter = %q", got)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			oldEndpoint := cloudazure.ManagementEndpoint
			cloudazure.ManagementEndpoint = server.URL
			t.Cleanup(func() { cloudazure.ManagementEndpoint = oldEndpoint })

			client := HealthClient{Azure: cloudazure.Azure{Cred: fakeCredential{}}}
			got, err := client.AllBackendsHealthy(t.Context(), "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Network/loadBalancers/lb", time.Date(2026, 9, 16, 18, 14, 0, 0, time.UTC))
			if err != nil {
				t.Fatalf("AllBackendsHealthy: %v", err)
			}
			if got != tt.want {
				t.Errorf("AllBackendsHealthy = %v, want %v", got, tt.want)
			}
		})
	}
}
