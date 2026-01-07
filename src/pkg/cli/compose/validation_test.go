package compose

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/smithy-go/ptr"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
)

func TestValidationAndConvert(t *testing.T) {
	oldTerm := term.DefaultTerm
	t.Cleanup(func() {
		term.DefaultTerm = oldTerm
	})

	t.Setenv("NODE_ENV", "if-you-see-this-env-was-used") // for interpolate/compose.yaml; should be ignored

	mockClient := &client.MockProvider{}
	listConfigNamesFunc := func(ctx context.Context) ([]string, error) {
		configs, err := mockClient.ListConfig(ctx, &defangv1.ListConfigsRequest{})
		if err != nil {
			return nil, err
		}
		return configs.Names, nil
	}

	testAllComposeFiles(t, func(t *testing.T, name, path string) {
		logs := new(bytes.Buffer)
		term.DefaultTerm = term.NewTerm(os.Stdin, logs, logs)

		options := LoaderOptions{ConfigPaths: []string{path}}
		loader := Loader{options: options}
		project, err := loader.LoadProject(t.Context())
		if strings.HasPrefix(name, "invalid-") {
			assert.Error(t, err, "Expected error for invalid compose file: %s", path)
			return
		}
		if err != nil {
			t.Fatal(err)
		}

		if err := FixupServices(t.Context(), mockClient, project, UploadModeIgnore); err != nil {
			t.Logf("Service conversion failed: %v", err)
			logs.WriteString("Error: " + err.Error() + "\n") // no coverage!
		}

		listConfigNames, err := listConfigNamesFunc(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateProjectConfig(project, listConfigNames); err != nil {
			t.Logf("Project config validation failed: %v", err)
			logs.WriteString("Error: " + err.Error() + "\n")
		}

		mode := modes.ModeAffordable
		if strings.Contains(path, "replicas") {
			mode = modes.ModeHighAvailability
		}
		if err := ValidateProject(project, mode); err != nil {
			t.Logf("Project validation failed: %v", err)
			logs.WriteString("Error: " + err.Error() + "\n") // no coverage!
		}

		// The order of the services is not guaranteed, so we sort the logs before comparing
		logLines := strings.SplitAfter(logs.String(), "\n")
		slices.Sort(logLines)
		logs = bytes.NewBufferString(strings.Join(logLines, ""))

		// Compare the logs with the warnings file
		if err := pkg.Compare(logs.Bytes(), path+".warnings"); err != nil {
			t.Error(err)
		}
	})
}

func TestValidateConfig(t *testing.T) {

	testProject := composeTypes.Project{
		Services: composeTypes.Services{},
	}

	t.Run("NOP", func(t *testing.T) {
		env := map[string]*string{
			"ENV_VAR": ptr.String("blah"),
		}
		testProject.Services["service1"] = composeTypes.ServiceConfig{Environment: env}

		if err := ValidateProjectConfig(&testProject, []string{}); err != nil {
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
		if err := ValidateProjectConfig(&testProject, []string{}); !errors.As(err, &missing) {
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

		if err := ValidateProjectConfig(&testProject, []string{CONFIG_VAR}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Missing interpolated variable", func(t *testing.T) {
		env := map[string]*string{
			"interpolated": ptr.String(`${CONFIG_VAR}`),
		}
		testProject.Services["service1"] = composeTypes.ServiceConfig{Environment: env}

		var missing ErrMissingConfig
		if err := ValidateProjectConfig(&testProject, []string{}); !errors.As(err, &missing) {
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
		wantValue bool
		wantErr   string
	}{
		{
			name: "sanity check",
			extension: map[string]any{
				"allow-downtime": true,
			},
			wantValue: true,
		},
		{
			name:      "empty",
			extension: nil,
			wantValue: false,
		},
		{
			name:      "true value",
			extension: true,
			wantValue: true,
		},
		{
			name:      "false value",
			extension: false,
			wantValue: false,
		},
		{
			name: "invalid downtime",
			extension: map[string]any{
				"allow-downtime": "abc",
			},
			wantErr: "'allow-downtime' must be a boolean",
		},
		{
			name:      "no options",
			extension: map[string]any{},
			wantValue: true,
		},
		{
			name:      "false string",
			extension: "false",
			wantValue: false,
		},
		{
			name:      "true string",
			extension: "true",
			wantValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := validateManagedStore(tt.extension)
			if err != nil {
				if tt.wantErr == "" {
					t.Fatalf("unexpected error: %v", err)
				} else if err.Error() != tt.wantErr {
					t.Fatalf("unexpected error: %v, expected: %s", err, tt.wantErr)
				}
			} else {
				if tt.wantErr != "" {
					t.Fatalf("expected error: %s, got: nil", tt.wantErr)
				}
				if val != tt.wantValue {
					t.Fatalf("unexpected value: %v, expected: %v", val, tt.wantValue)
				}
			}
		})
	}
}
