package compose

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/smithy-go/ptr"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

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

func TestValidationAndConvert(t *testing.T) {
	oldTerm := term.DefaultTerm
	t.Cleanup(func() {
		term.DefaultTerm = oldTerm
	})

	t.Setenv("NODE_ENV", "if-you-see-this-env-was-used") // for interpolate/compose.yaml; should be ignored

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
		logs := new(bytes.Buffer)
		term.DefaultTerm = term.NewTerm(os.Stdin, logs, logs)

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
		logLines := strings.SplitAfter(logs.String(), "\n")
		slices.Sort(logLines)
		logs = bytes.NewBufferString(strings.Join(logLines, ""))

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

func TestManagedStoreParams(t *testing.T) {
	tests := []struct {
		name      string
		extension any
		errors    []string
	}{
		{
			name: "sanity check",
			extension: map[string]any{
				"allow-downtime": true,
			},
			errors: []string{},
		},
		{
			name:      "empty",
			extension: nil,
			errors:    []string{},
		},
		{
			name:      "true value",
			extension: true,
			errors:    []string{},
		},
		{
			name:      "false value",
			extension: false,
			errors:    []string{},
		},
		{
			name: "invalid downtime",
			extension: map[string]any{
				"allow-downtime": "abc",
			},
			errors: []string{"'allow-downtime' must be a boolean"},
		},
		{
			name:      "no options",
			extension: map[string]any{},
			errors:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateManagedStore(tt.extension); err != nil {
				if len(tt.errors) == 0 {
					t.Fatalf("ValidateManagedStore() unexpected err = %v", err)
				}

				for _, errMsg := range tt.errors {
					if !strings.Contains(err.Error(), errMsg) {
						t.Fatalf("ValidateManagedStore() = %v, want %v", err.Error(), tt.errors)
					}
				}
			}
		})
	}
}
