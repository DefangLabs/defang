package compose

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/smithy-go/ptr"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func TestValidationAndConvert(t *testing.T) {
	oldTerm := term.DefaultTerm
	t.Cleanup(func() {
		term.DefaultTerm = oldTerm
	})

	t.Setenv("NODE_ENV", "test") // for interpolate/compose.yaml

	mockClient := validationMockProvider{
		configs: []string{"CONFIG1", "CONFIG2", "dummy", "ENV1", "SENSITIVE_DATA"},
	}
	listConfigNamesFunc := func(ctx context.Context) ([]string, error) {
		configs, err := mockClient.ListConfig(ctx, &defangv1.ListConfigsRequest{})
		if err != nil {
			return nil, err
		}

		return configs.Names, nil
	}

	testRunCompose(t, func(t *testing.T, path string) {
		t.Helper()
		logs := new(bytes.Buffer)
		term.DefaultTerm = term.NewTerm(logs, logs)

		options := LoaderOptions{ConfigPaths: []string{path}}
		loader := Loader{options: options}
		project, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		if err := ValidateProjectConfig(context.Background(), project, listConfigNamesFunc); err != nil {
			t.Logf("Project config validation failed: %v", err)
			logs.WriteString(err.Error() + "\n")
		}

		if err := ValidateProject(project); err != nil {
			t.Logf("Project validation failed: %v", err)
			logs.WriteString(err.Error() + "\n")
		}

		if err := FixupServices(context.Background(), mockClient, project, UploadModeIgnore); err != nil {
			t.Logf("Service conversion failed: %v", err)
			logs.WriteString(err.Error() + "\n")
		}

		// The order of the services is not guaranteed, so we sort the logs before comparing
		logLines := strings.Split(strings.Trim(logs.String(), "\n"), "\n")
		slices.Sort(logLines)
		logs = bytes.NewBufferString(strings.Join(logLines, "\n"))

		// Compare the logs with the warnings file
		if err := compare(logs.Bytes(), path+".warnings"); err != nil {
			t.Error(err)
		}
	})
}

func makeListConfigNamesFunc(configs ...string) func(context.Context) ([]string, error) {
	return func(context.Context) ([]string, error) {
		return configs, nil
	}
}

func TestValidateConfig(t *testing.T) {
	ctx := context.Background()

	testProject := composeTypes.Project{
		Services: composeTypes.Services{},
	}

	t.Run("NOP", func(t *testing.T) {
		env := map[string]*string{
			"ENV_VAR": ptr.String("blah"),
		}
		testProject.Services["service1"] = composeTypes.ServiceConfig{Environment: env}

		if err := ValidateProjectConfig(ctx, &testProject, makeListConfigNamesFunc()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Missing Config", func(t *testing.T) {
		env := map[string]*string{
			"ENV_VAR": ptr.String("blah"),
			"ASD":     nil,
			"BSD":     nil,
			"CSD":     nil,
		}
		testProject.Services["service1"] = composeTypes.ServiceConfig{Environment: env}

		var missing ErrMissingConfig
		if err := ValidateProjectConfig(ctx, &testProject, makeListConfigNamesFunc()); !errors.As(err, &missing) {
			t.Fatalf("expected ErrMissingConfig, got: %v", err)
		} else {
			if len(missing) != 3 {
				t.Fatalf("unexpected error: number of missing, got: %d expected 3", len(missing))
			}

			for index, name := range []string{"ASD", "BSD", "CSD"} {
				if missing[index] != name {
					t.Fatalf("unexpected error: missing, got: %s expected ASD", missing[index])
				}
			}
		}
	})

	t.Run("Valid Config", func(t *testing.T) {
		const CONFIG_VAR = "CONFIG_VAR"
		env := map[string]*string{
			"ENV_VAR":  ptr.String("blah"),
			CONFIG_VAR: nil,
		}
		testProject.Services["service1"] = composeTypes.ServiceConfig{Environment: env}

		if err := ValidateProjectConfig(ctx, &testProject, makeListConfigNamesFunc(CONFIG_VAR)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Missing interpolated variable", func(t *testing.T) {
		env := map[string]*string{
			"interpolated": ptr.String(`${CONFIG_VAR}`),
		}
		testProject.Services["service1"] = composeTypes.ServiceConfig{Environment: env}

		var missing ErrMissingConfig
		if err := ValidateProjectConfig(ctx, &testProject, makeListConfigNamesFunc()); !errors.As(err, &missing) {
			t.Fatalf("expected ErrMissingConfig, got: %v", err)
		} else {
			if len(missing) != 1 {
				t.Fatalf("unexpected error: number of missing, got: %d expected 1", len(missing))
			}

			if missing[0] != "CONFIG_VAR" {
				t.Fatalf("unexpected error: missing, got: %s expected CONFIG_VAR", missing[0])
			}
		}
	})
}

type validationMockProvider struct {
	client.Provider
	configs []string
}

func (m validationMockProvider) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	return &defangv1.Secrets{
		Names:   m.configs,
		Project: "mock-project",
	}, nil
}

func (m validationMockProvider) ServiceDNS(name string) string {
	return "mock-" + name
}
