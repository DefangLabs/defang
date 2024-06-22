package compose

import (
	"bytes"
	"context"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
)

func TestProjectValidationNetworks(t *testing.T) {
	var warnings bytes.Buffer
	logrus.SetOutput(&warnings)

	loader := Loader{"../../../tests/testproj/compose.yaml"}
	p, err := loader.LoadCompose(context.Background())
	if err != nil {
		t.Fatalf("LoadCompose() failed: %v", err)
	}

	dfnx := p.Services["dfnx"]
	dfnx.Networks = map[string]*types.ServiceNetworkConfig{"invalid-network-name": nil}
	p.Services["dfnx"] = dfnx
	if err := ValidateProject(p); err != nil {
		t.Errorf("Invalid network name should not be an error: %v", err)
	}
	if !bytes.Contains(warnings.Bytes(), []byte(`network \"invalid-network-name\" is not defined`)) {
		t.Errorf("Invalid network name should trigger a warning: %v", warnings.String())
	}

	warnings.Reset()
	dfnx.Networks = map[string]*types.ServiceNetworkConfig{"public": nil}
	p.Services["dfnx"] = dfnx
	if err := ValidateProject(p); err != nil {
		t.Errorf("public network name should not be an error: %v", err)
	}
	if !bytes.Contains(warnings.Bytes(), []byte(`network \"public\" is not defined`)) {
		t.Errorf("missing public network in global networks section should trigger a warning: %v", warnings.String())
	}

	warnings.Reset()
	p.Networks["public"] = types.NetworkConfig{}
	if err := ValidateProject(p); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if bytes.Contains(warnings.Bytes(), []byte(`network \"public\" is not defined`)) {
		t.Errorf("When public network is defined globally should not trigger a warning when public network is used")
	}
}

func TestProjectValidationServiceName(t *testing.T) {
	loader := Loader{"../../../tests/testproj/compose.yaml"}
	p, err := loader.LoadCompose(context.Background())
	if err != nil {
		t.Fatalf("LoadCompose() failed: %v", err)
	}

	if err := ValidateProject(p); err != nil {
		t.Fatalf("Project validation failed: %v", err)
	}

	svc := p.Services["dfnx"]
	longName := "aVeryLongServiceNameThatIsDefinitelyTooLongThatWillCauseAnError"
	svc.Name = longName
	p.Services[longName] = svc

	if err := ValidateProject(p); err == nil {
		t.Fatalf("Long project name should be an error")
	}
}

func TestProjectValidationNoDeploy(t *testing.T) {
	loader := Loader{"../../../tests/testproj/compose.yaml"}
	p, err := loader.LoadCompose(context.Background())
	if err != nil {
		t.Fatalf("LoadCompose() failed: %v", err)
	}

	dfnx := p.Services["dfnx"]
	dfnx.Deploy = nil
	p.Services["dfnx"] = dfnx
	if err := ValidateProject(p); err != nil {
		t.Errorf("No deploy section should not be an error: %v", err)
	}
}
