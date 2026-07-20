package compose

import (
	"bytes"
	"context"
	"errors"
	"io"
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

		mode := modes.RecipeAffordable
		if strings.Contains(path, "replicas") {
			mode = modes.RecipeHighAvailability
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

func TestValidateModelConfig(t *testing.T) {
	tests := []struct {
		name        string
		model       string
		configNames []string
		want        []string
	}{
		{
			name:  "concrete model is valid",
			model: "bedrock/anthropic.claude-sonnet-5",
		},
		{
			name:  "unresolved model interpolation fails",
			model: "${MODEL_NAME}",
			want:  []string{"MODEL_NAME"},
		},
		{
			name:  "unresolved unbraced model interpolation fails",
			model: "$MODEL_NAME",
			want:  []string{"MODEL_NAME"},
		},
		{
			name:        "Defang config does not make model interpolation valid",
			model:       "${MODEL_NAME}",
			configNames: []string{"MODEL_NAME"},
			want:        []string{"MODEL_NAME"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := &composeTypes.Project{
				Services: composeTypes.Services{},
				Models: map[string]composeTypes.ModelConfig{
					"llm": {Model: tt.model},
				},
			}

			err := ValidateProjectConfig(project, tt.configNames)
			if len(tt.want) == 0 {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var interpolations ErrConfigInterpolationInModels
			if !errors.As(err, &interpolations) {
				t.Fatalf("expected ErrConfigInterpolationInModels, got: %v", err)
			}
			assert.Equal(t, tt.want, []string(interpolations))
			assert.Contains(t, err.Error(), "define them in `.env`")
			assert.Contains(t, err.Error(), "chat-default, chat-large, embedding-default")
		})
	}

	t.Run("deprecated provider model is also validated", func(t *testing.T) {
		project := &composeTypes.Project{Services: composeTypes.Services{
			"chat": {
				Provider: &composeTypes.ServiceProviderConfig{
					Type:    "model",
					Options: map[string][]string{"model": {"${MODEL_NAME}"}},
				},
			},
		}}

		err := ValidateProjectConfig(project, []string{"MODEL_NAME"})
		var interpolations ErrConfigInterpolationInModels
		if !errors.As(err, &interpolations) {
			t.Fatalf("expected ErrConfigInterpolationInModels, got: %v", err)
		}
		assert.Equal(t, []string{"MODEL_NAME"}, []string(interpolations))
	})
}

func TestValidateBuildArgs(t *testing.T) {
	oldTerm := term.DefaultTerm
	t.Cleanup(func() { term.DefaultTerm = oldTerm })
	term.DefaultTerm = term.NewTerm(os.Stdin, io.Discard, io.Discard)

	newProject := func(args map[string]*string) *composeTypes.Project {
		return &composeTypes.Project{
			Services: composeTypes.Services{
				"backend": composeTypes.ServiceConfig{
					Name:  "backend",
					Build: &composeTypes.BuildConfig{Context: ".", Args: args},
				},
			},
		}
	}

	tests := []struct {
		name    string
		args    map[string]*string
		wantBad []string // non-empty means we expect ErrConfigInterpolationInBuildArgs with these paths
	}{
		{
			name: "concrete value is allowed",
			args: map[string]*string{"MY_APP_URL": ptr.String("https://example.com")},
		},
		{
			name: "nil value (passed from build environment) is allowed",
			args: map[string]*string{"MY_APP_URL": nil},
		},
		{
			name: "escaped dollar is allowed",
			args: map[string]*string{"PRICE": ptr.String("$$5.00")},
		},
		{
			name:    "unresolved curly interpolation is rejected",
			args:    map[string]*string{"MY_APP_URL": ptr.String("${MY_APP_URL}")},
			wantBad: []string{"backend.build.args.MY_APP_URL"},
		},
		{
			name:    "unresolved bare interpolation is rejected",
			args:    map[string]*string{"MY_APP_URL": ptr.String("$MY_APP_URL")},
			wantBad: []string{"backend.build.args.MY_APP_URL"},
		},
		{
			name:    "interpolation embedded in a larger value is rejected",
			args:    map[string]*string{"URL": ptr.String("https://${HOST}/api")},
			wantBad: []string{"backend.build.args.URL"},
		},
		{
			name: "multiple offending args are all reported and sorted",
			args: map[string]*string{
				"B_URL": ptr.String("${B}"),
				"A_URL": ptr.String("${A}"),
			},
			wantBad: []string{"backend.build.args.A_URL", "backend.build.args.B_URL"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProject(newProject(tt.args), modes.RecipeAffordable)
			var badArgs ErrConfigInterpolationInBuildArgs
			if len(tt.wantBad) == 0 {
				if errors.As(err, &badArgs) {
					t.Fatalf("unexpected ErrConfigInterpolationInBuildArgs: %v", err)
				}
				return
			}
			if !errors.As(err, &badArgs) {
				t.Fatalf("expected ErrConfigInterpolationInBuildArgs, got: %v", err)
			}
			assert.Equal(t, tt.wantBad, []string(badArgs))
		})
	}
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

func TestServiceExtensionWarnings(t *testing.T) {
	oldTerm := term.DefaultTerm
	t.Cleanup(func() { term.DefaultTerm = oldTerm })

	tests := []struct {
		name        string
		extension   string
		value       any
		wantWarning bool
	}{
		// Read by the CLI itself.
		{"x-defang-llm is recognized", "x-defang-llm", true, false},
		// Consumed only by the CD provider, but valid and passed through.
		{"x-defang-policies is recognized", "x-defang-policies", []any{"arn:aws:iam::aws:policy/AdministratorAccess"}, false},
		{"x-defang-aliases is recognized", "x-defang-aliases", map[string]any{"cluster": "urn:pulumi:stack::proj::aws:ecs/cluster:Cluster::c"}, false},
		// Genuinely unknown extensions still warn.
		{"unknown extension warns", "x-defang-bogus", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			term.DefaultTerm = term.NewTerm(os.Stdin, &out, &out)

			svc := &composeTypes.ServiceConfig{
				Name:       "svc",
				Image:      "nginx",
				Extensions: composeTypes.Extensions{tt.extension: tt.value},
			}
			project := &composeTypes.Project{Services: composeTypes.Services{"svc": *svc}}

			if err := validateService(svc, project, modes.RecipeAffordable); err != nil {
				t.Fatalf("validateService returned error: %v", err)
			}

			warned := strings.Contains(out.String(), `unsupported compose extension: "`+tt.extension+`"`)
			assert.Equal(t, tt.wantWarning, warned, "term output: %q", out.String())
		})
	}
}
