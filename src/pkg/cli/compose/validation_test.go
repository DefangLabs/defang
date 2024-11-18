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

func TestXDefangPostgres(t *testing.T) {
	t.Run("verify empty definition", func(t *testing.T) {
		service := composeTypes.ServiceConfig{
			Extensions: map[string]interface{}{
				"x-defang-postgres": nil,
			}}

		postgres, ok := service.Extensions["x-defang-postgres"]
		if !ok {
			t.Fatal("x-defang-postgres extension not found")
		}

		if err := ValidatePostgres(postgres); err != nil {
			t.Fatalf("ValidateProtgresService() failed: %v", err)
		}
	})

	t.Run("verify bool value", func(t *testing.T) {
		service := composeTypes.ServiceConfig{
			Extensions: map[string]interface{}{
				"x-defang-postgres": true,
			}}

		postgres, ok := service.Extensions["x-defang-postgres"]
		if !ok {
			t.Fatal("x-defang-postgres extension not found")
		}

		if err := ValidatePostgres(postgres); err != nil {
			t.Fatalf("ValidateProtgresService() failed: %v", err)
		}
	})

	t.Run("verify full definition", func(t *testing.T) {
		service := composeTypes.ServiceConfig{
			Extensions: composeTypes.Extensions{
				"x-defang-postgres": map[string]any{
					"maintenance": map[string]any{
						"day-of-week": "Thursday",
						"duration":    1,
						"start-time":  "00:00",
					},
					"retention": map[string]any{
						"number-of-days-to-keep": 7,
						"restore-on-startup":     true,
						"save-on-deprovisioning": true,
					},
				},
			}}

		postgres, ok := service.Extensions["x-defang-postgres"]
		if !ok {
			t.Fatal("x-defang-postgres extension not found")
		}

		if err := ValidatePostgres(postgres); err != nil {
			t.Fatalf("ValidateProtgresService() failed: %v", err)
		}
	})
}

func TestXDefangPostgresParams(t *testing.T) {
	tests := []struct {
		name      string
		extension map[string]any
		errors    []string
	}{
		{
			name: "invalid maintentance and retention",
			extension: map[string]any{
				"maintenance": "abc",
				"retention":   123,
			},
			errors: []string{"'maintenance' must contain 'day-of-week', 'duration', and 'start-time' fields",
				"'retention' must contain 'number-of-days-to-keep', 'restore-on-startup', and 'save-on-deprovisioning' fields"},
		},
		{
			name: "invalid day",
			extension: map[string]any{
				"maintenance": map[string]any{"day-of-week": "Thurs", "duration": 1, "start-time": "00:00"},
				"retention":   nil,
			},
			errors: []string{"'day-of-week' must be a day of the week"},
		},
		{
			name: "invalid duration: not a number",
			extension: map[string]any{
				"maintenance": map[string]any{"day-of-week": "Thursday", "duration": "A", "start-time": "00:00"},
				"retention":   nil,
			},
			errors: []string{"'duration' must be a number"},
		},
		{
			name: "invalid start-time",
			extension: map[string]any{
				"maintenance": map[string]any{"day-of-week": "Thursday", "duration": 1, "start-time": "25:77"},
				"retention":   nil,
			},
			errors: []string{"'start-time' must be a valid time in \"HH:MM\" format"},
		},
		{
			name: "invalid start-time type",
			extension: map[string]any{
				"maintenance": map[string]any{"day-of-week": "Thursday", "duration": 1, "start-time": 123},
				"retention":   nil,
			},
			errors: []string{"'start-time' must be a valid time in \"HH:MM\" format"},
		},
		{
			name: "missing day-of-week",
			extension: map[string]any{
				"maintenance": map[string]any{"duration": 1, "start-time": "20:30"},
				"retention":   nil,
			},
			errors: []string{"missing 'day-of-week' field"},
		},
		{
			name: "missing duration",
			extension: map[string]any{
				"maintenance": map[string]any{"day-of-week": "Thursday", "start-time": "20:30"},
				"retention":   nil,
			},
			errors: []string{"missing 'duration' field"},
		},
		{
			name: "missing start-time",
			extension: map[string]any{
				"maintenance": map[string]any{"day-of-week": "Thursday", "duration": 2},
				"retention":   nil,
			},
			errors: []string{"missing 'start-time' field"},
		},
		{
			name: "missing day-of-week and start-time",
			extension: map[string]any{
				"maintenance": map[string]any{"duration": 2, "start-time": "00:00"},
				"retention":   nil,
			},
			errors: []string{"missing 'day-of-week' field"},
		},
		{
			name: "invalid number-of-days-to-keep",
			extension: map[string]any{
				"maintenance": nil,
				"retention":   map[string]any{"number-of-days-to-keep": "A", "restore-on-startup": true, "save-on-deprovisioning": true},
			},
			errors: []string{"'number-of-days-to-keep' must be a number"},
		},
		{
			name: "invalid restore-on-startup",
			extension: map[string]any{
				"maintenance": nil,
				"retention":   map[string]any{"number-of-days-to-keep": 1, "restore-on-startup": "abc", "save-on-deprovisioning": true},
			},
			errors: []string{"'restore-on-startup' must be set to true or false"},
		},
		{
			name: "invalid save-on-deprovisioning",
			extension: map[string]any{
				"maintenance": nil,
				"retention":   map[string]any{"number-of-days-to-keep": 1, "restore-on-startup": true, "save-on-deprovisioning": "abc"},
			},
			errors: []string{"'save-on-deprovisioning' must be set to true or false"},
		},
		{
			name: "missing number-of-days-to-keep",
			extension: map[string]any{
				"maintenance": nil,
				"retention":   map[string]any{"restore-on-startup": true, "save-on-deprovisioning": true},
			},
			errors: []string{"missing 'number-of-days-to-keep' field"},
		},
		{
			name: "missing number-of-days-to-keep and restore-on-startup",
			extension: map[string]any{
				"maintenance": nil,
				"retention":   map[string]any{"save-on-deprovisioning": true},
			},
			errors: []string{"missing 'number-of-days-to-keep' field", "missing 'restore-on-startup' field"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePostgres(tt.extension); err != nil {
				var errPostgres *ErrPostgresParam
				if !errors.As(err, &errPostgres) {
					t.Fatalf("unexpected error: %v", err)
				}

				for _, errMsg := range tt.errors {
					if !slices.Contains(*errPostgres, errMsg) {
						t.Fatalf("ValidatePostgresParams() = %v, want %v", errPostgres.Error(), tt.errors)
					}
				}

				if len(tt.errors) != len(*errPostgres) {
					t.Fatalf("expected %d errors but got %d", len(tt.errors), len(*errPostgres))
				}
			}
		})
	}
}
