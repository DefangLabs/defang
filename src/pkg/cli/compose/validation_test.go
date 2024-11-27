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

func TestXDefangManaged(t *testing.T) {
	var serviceExtensionName = []string{"x-defang-post", "x-defang-redis"}
	for _, extensionName := range serviceExtensionName {
		t.Run("verify empty definition", func(t *testing.T) {
			service := composeTypes.ServiceConfig{
				Extensions: map[string]interface{}{
					extensionName: nil,
				}}

			redis, ok := service.Extensions[extensionName]
			if !ok {
				t.Fatalf("%s extension not found", extensionName)
			}

			if err := ValidateManagedStore(redis); err != nil {
				t.Fatalf("ValidateRedis() failed: %v", err)
			}
		})

		t.Run("verify bool value", func(t *testing.T) {
			service := composeTypes.ServiceConfig{
				Extensions: map[string]interface{}{
					extensionName: true,
				}}

			redis, ok := service.Extensions[extensionName]
			if !ok {
				t.Fatalf("%s extension not found", extensionName)
			}

			if err := ValidateManagedStore(redis); err != nil {
				t.Fatalf("ValidateRedis() failed: %v", err)
			}
		})

		t.Run("verify full definition", func(t *testing.T) {
			service := composeTypes.ServiceConfig{
				Extensions: composeTypes.Extensions{
					extensionName: map[string]any{
						"allow-downtime": true,
						"retention": map[string]any{
							"retention-period":            "7d",
							"final-snapshot-name":         "final-snapshot",
							"snapshot-to-load-on-startup": "load-snapsot",
						},
					},
				}}

			postgres, ok := service.Extensions[extensionName]
			if !ok {
				t.Fatalf("%s extension not found", extensionName)
			}

			if err := ValidateManagedStore(postgres); err != nil {
				t.Fatalf("ValidateRedis() failed: %v", err)
			}
		})
	}
}

func TestManagedStoreParams(t *testing.T) {
	tests := []struct {
		name      string
		extension map[string]any
		errors    []string
	}{
		{
			name: "invalid maintentance and retention",
			extension: map[string]any{
				"allow-downtime": "abc",
				"retention":      123,
			},
			errors: []string{"error found: 'allow-downtime' must be a boolean",
				"error found: 'retention' should contain 'final-snapshot-name' field"},
		},
		{
			name: "valid allow-downtime",
			extension: map[string]any{
				"allow-downtime": false,
				"retention":      nil,
			},
			errors: []string{},
		},
		{
			name: "invalid retention-period as number",
			extension: map[string]any{
				"retention": map[string]any{
					"retention-period":            7,
					"final-snapshot-name":         "final-snapshot",
					"snapshot-to-load-on-startup": "old-snapshot",
				},
			},
			errors: []string{"'retention-period' must be string in the format of '<number of days>d'"},
		},
		{
			name: "invalid retention-period",
			extension: map[string]any{
				"retention": map[string]any{
					"retention-period":            "a",
					"final-snapshot-name":         "final-snapshot",
					"snapshot-to-load-on-startup": "old-snapshot",
				},
			},
			errors: []string{"'retention-period' must be string in the format of '<number of days>d'"},
		},
		{
			name: "invalid final-snapshot-name",
			extension: map[string]any{
				"retention": map[string]any{
					"retention-period":            "7d",
					"final-snapshot-name":         1234,
					"snapshot-to-load-on-startup": "old-snapshot",
				},
			},
			errors: []string{"'final-snapshot-name' must be a string"},
		},
		{
			name: "invalid snapshot-to-load-on-startup",
			extension: map[string]any{
				"retention": map[string]any{
					"retention-period":            "7d",
					"final-snapshot-name":         "final-snapshot",
					"snapshot-to-load-on-startup": 123,
				},
			},
			errors: []string{"'snapshot-to-load-on-startup' must be a string"},
		},
		{
			name: "missing retention-period",
			extension: map[string]any{
				"retention": map[string]any{
					"final-snapshot-name":         "final-snapshot",
					"snapshot-to-load-on-startup": "load-snapshot",
				},
			},
			errors: []string{"'retention-period' should be defined when 'final-snapshot-name' is set"},
		},
		{
			name: "no fields",
			extension: map[string]any{
				"retention": map[string]any{},
			},
			errors: []string{},
		},
		{
			name: "missing final-snapshot-name and snapshot-to-load-on-startup",
			extension: map[string]any{
				"retention": map[string]any{
					"retention-period": "7d",
				},
			},
			errors: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateManagedStore(tt.extension); err != nil {
				for _, errMsg := range tt.errors {
					if !strings.Contains(err.Error(), errMsg) {
						t.Fatalf("ValidatePostgresParams() = %v, want %v", err.Error(), tt.errors)
					}
				}
			}
		})
	}
}
