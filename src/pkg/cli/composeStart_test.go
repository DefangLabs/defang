package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestComposeStart(t *testing.T) {
	DoDryRun = true
	defer func() { DoDryRun = false }()

	loader := ComposeLoader{"../../tests/testproj/compose.yaml"}
	proj, err := loader.LoadCompose()
	if err != nil {
		t.Fatalf("LoadCompose() failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	_, err = ComposeStart(context.Background(), client.MockClient{UploadUrl: server.URL + "/", Project: proj}, false)
	if !errors.Is(err, ErrDryRun) {
		t.Fatalf("ComposeStart() failed: %v", err)
	}
}

func TestComposeFixupEnv(t *testing.T) {
	loader := ComposeLoader{"../../tests/fixupenv/compose.yaml"}
	proj, err := loader.LoadCompose()
	if err != nil {
		t.Fatalf("LoadCompose() failed: %v", err)
	}

	services, err := convertServices(context.Background(), client.MockClient{}, proj.Services, false)
	if err != nil {
		t.Fatalf("convertServices() failed: %v", err)
	}
	ui := slices.IndexFunc(services, func(s *defangv1.Service) bool { return s.Name == "ui" })
	if ui < 0 {
		t.Fatalf("convertServices() failed: expected service named 'ui'")
	}

	const expected = "http://mistral:8000"
	got := services[ui].Environment["API_URL"]
	if got != expected {
		t.Errorf("convertServices() failed: expected API_URL=%s, got %s", expected, got)
	}

	const sensitiveKey = "SENSITIVE_DATA"
	_, ok := services[ui].Environment[sensitiveKey]
	if ok {
		t.Errorf("convertServices() failed: , %s found in environment map but should not be.", sensitiveKey)
	}

	found := false
	for _, value := range services[ui].Secrets {
		if value.Source == sensitiveKey {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("convertServices() failed: unable to find sensitive config variable %s", sensitiveKey)
	}
}
