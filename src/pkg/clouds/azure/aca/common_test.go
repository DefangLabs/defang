package aca

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cloudazure "github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

func newTestContainerApp() *ContainerApp {
	return &ContainerApp{
		Azure: cloudazure.Azure{
			SubscriptionID: "sub",
			Location:       cloudazure.LocationWestUS2,
		},
		ResourceGroup: "rg",
	}
}

func TestContainerAppGetAuthToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "Microsoft.App/containerApps/myapp/getAuthToken") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"properties":{"token":"ca-tok"}}`))
	}))
	defer srv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, srv.URL, "")

	c := newTestContainerApp()
	tok, err := c.getAuthToken(context.Background(), "myapp")
	if err != nil {
		t.Fatalf("getAuthToken: %v", err)
	}
	if tok != "ca-tok" {
		t.Errorf("token = %q", tok)
	}
}

func TestContainerAppGetEventStreamBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"properties":{"eventStreamEndpoint":"https://westus2.ms.management.azure.com/subscriptions/x/foo"}}`))
	}))
	defer srv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, srv.URL, "")

	c := newTestContainerApp()
	base, err := c.getEventStreamBase(context.Background(), "myapp")
	if err != nil {
		t.Fatalf("getEventStreamBase: %v", err)
	}
	if base != "https://westus2.ms.management.azure.com" {
		t.Errorf("base = %q", base)
	}
}

func TestContainerAppGetEventStreamBaseMalformed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"properties":{"eventStreamEndpoint":"no-subscription-path"}}`))
	}))
	defer srv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, srv.URL, "")

	c := newTestContainerApp()
	if _, err := c.getEventStreamBase(context.Background(), "myapp"); err == nil {
		t.Error("expected error for malformed eventStreamEndpoint")
	}
}

func TestContainerAppGetEventStreamBaseHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, srv.URL, "")

	c := newTestContainerApp()
	if _, err := c.getEventStreamBase(context.Background(), "myapp"); err == nil {
		t.Error("expected HTTP error")
	}
}

func TestContainerAppGetEventStreamBaseBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, srv.URL, "")

	c := newTestContainerApp()
	if _, err := c.getEventStreamBase(context.Background(), "myapp"); err == nil {
		t.Error("expected decode error")
	}
}

func TestContainerAppGetEventStreamBaseArmError(t *testing.T) {
	useFakeCred(t, "", errors.New("no arm token"))
	c := newTestContainerApp()
	if _, err := c.getEventStreamBase(context.Background(), "myapp"); err == nil {
		t.Error("expected ArmToken error")
	}
}

func TestContainerAppNewClients(t *testing.T) {
	useFakeCred(t, "tok", nil)
	c := newTestContainerApp()
	if cli, err := c.newContainerAppsClient(); err != nil || cli == nil {
		t.Errorf("newContainerAppsClient: %v, client=%v", err, cli)
	}
	if cli, err := c.newReplicasClient(); err != nil || cli == nil {
		t.Errorf("newReplicasClient: %v, client=%v", err, cli)
	}
}
