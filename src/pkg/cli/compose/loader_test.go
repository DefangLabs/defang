package compose

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
)

func TestLoader(t *testing.T) {
	testAllComposeFiles(t, func(t *testing.T, name, path string) {
		loader := NewLoader(WithPath(path))
		proj, err := loader.LoadProject(t.Context())
		if strings.HasPrefix(name, "invalid-") {
			assert.Error(t, err, "Expected error for invalid compose file: %s", path)
			return
		}
		if err != nil {
			t.Fatal(err)
		}

		yaml, err := MarshalYAML(proj)
		if err != nil {
			t.Fatal(err)
		}

		// Compare the output with the golden file
		if err := pkg.Compare(yaml, path+".golden"); err != nil {
			t.Error(err)
		}
	})
}

func testAllComposeFiles(t *testing.T, f func(t *testing.T, name, path string)) {
	t.Helper()

	composeRegex := regexp.MustCompile(`^(?i)(docker-)?compose.ya?ml$`)
	err := filepath.WalkDir("../../../testdata", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !composeRegex.MatchString(d.Name()) {
			return err
		}

		t.Run(path, func(t *testing.T) {
			t.Helper()
			t.Log(path)
			f(t, filepath.Base(filepath.Dir(path)), path)
		})
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func TestHasSubstitution(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "no substitution",
			input:    "${var}",
			expected: false,
		},
		{
			name:     "substitution",
			input:    "${var-def}",
			expected: true,
		},
		{
			name:     "escaped substitution",
			input:    "$${var-def}",
			expected: false,
		},
		{
			name:     "escaped dollar and substitution",
			input:    "$${var+def}",
			expected: false,
		},
		// following test not supported yet
		// {
		// 	name:     "escaped dollar and escaped substitution",
		// 	input:    "$$${var?def}",
		// 	expected: true,
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if hasSubstitution(tt.input, "var") != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, !tt.expected)
			}
		})
	}
}

func TestComposeEnv(t *testing.T) {
	t.Setenv("COMPOSE_PROJECT_NAME", "env_project_name")
	t.Setenv("COMPOSE_PATH_SEPARATOR", "|")
	t.Setenv("COMPOSE_FILE", "../../../testdata/multiple/compose1.yaml|../../../testdata/multiple/compose2.yaml")
	t.Setenv("COMPOSE_DISABLE_ENV_FILE", "1")

	loader := NewLoader()
	p, err := loader.LoadProject(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "env_project_name", p.Name)
	assert.Len(t, p.Services, 2)
	assert.Equal(t, types.NewMappingWithEquals([]string{"A=${A}"}), p.Services["service1"].Environment)
}

func TestWithEnvFiles(t *testing.T) {
	// A minimal project whose interpolation depends on env-file values.
	const composeYAML = `name: envfiletest
services:
  web:
    image: alpine
    environment:
      - GREETING=${GREETING}
      - SOURCE=${SOURCE}
`
	dir := t.TempDir()
	writeFile := func(name, content string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	composePath := writeFile("compose.yaml", composeYAML)
	// The default .env sits next to the compose file and is loaded when no --env-file is given.
	writeFile(".env", "GREETING=from_dotenv\nSOURCE=default\n")
	// The explicit env-file only defines GREETING.
	prodEnv := writeFile("prod.env", "GREETING=from_prod\n")
	// A second env-file to verify multiple --env-file values merge (last wins for duplicates).
	extraEnv := writeFile("extra.env", "SOURCE=from_extra\n")

	tests := []struct {
		name     string
		envFiles []string
		expected map[string]string
	}{
		{
			name:     "default .env is used when no env-file is given",
			envFiles: nil,
			expected: map[string]string{"GREETING": "from_dotenv", "SOURCE": "default"},
		},
		{
			name:     "explicit env-file overrides the default .env",
			envFiles: []string{prodEnv},
			// SOURCE is absent from prod.env and the default .env is no longer
			// loaded, so it stays unresolved for resolution later by CD.
			expected: map[string]string{"GREETING": "from_prod", "SOURCE": "${SOURCE}"},
		},
		{
			name:     "multiple env-files are merged",
			envFiles: []string{prodEnv, extraEnv},
			expected: map[string]string{"GREETING": "from_prod", "SOURCE": "from_extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader(WithPath(composePath), WithEnvFiles(tt.envFiles...))
			p, err := loader.LoadProject(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			env := p.Services["web"].Environment
			for k, want := range tt.expected {
				got := env[k]
				if got == nil {
					t.Errorf("environment variable %q not found", k)
					continue
				}
				assert.Equal(t, want, *got, "environment variable %q", k)
			}
		})
	}
}

// TestWithEnvFilesFromEnvVar covers the COMPOSE_ENV_FILES fallback. This is the
// path a stack file uses: LoadStackEnv sets COMPOSE_ENV_FILES in the process env
// before the project is loaded, so resolving it here (rather than up front) is
// what makes a stack-scoped env file take effect. Regression test for that bug.
func TestWithEnvFilesFromEnvVar(t *testing.T) {
	const composeYAML = `name: envfilevartest
services:
  web:
    image: alpine
    environment:
      - GREETING=${GREETING}
`
	dir := t.TempDir()
	writeFile := func(name, content string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	composePath := writeFile("compose.yaml", composeYAML)
	writeFile(".env", "GREETING=from_dotenv\n")
	stackEnv := writeFile("stack.env", "GREETING=from_stack\n")
	flagEnv := writeFile("flag.env", "GREETING=from_flag\n")

	tests := []struct {
		name     string
		envFiles []string // explicit --env-file (WithEnvFiles)
		envVar   string   // COMPOSE_ENV_FILES; "" means unset
		expected string
	}{
		{
			name:     "COMPOSE_ENV_FILES is honored when no explicit env-file is set",
			envVar:   stackEnv,
			expected: "from_stack",
		},
		{
			name:     "explicit env-file overrides COMPOSE_ENV_FILES",
			envFiles: []string{flagEnv},
			envVar:   stackEnv,
			expected: "from_flag",
		},
		{
			name:     "default .env is used when neither is set",
			expected: "from_dotenv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVar != "" {
				t.Setenv(composeEnvFilesEnvVar, tt.envVar)
			} else {
				os.Unsetenv(composeEnvFilesEnvVar)
			}
			loader := NewLoader(WithPath(composePath), WithEnvFiles(tt.envFiles...))
			p, err := loader.LoadProject(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			got := p.Services["web"].Environment["GREETING"]
			if got == nil {
				t.Fatal("environment variable GREETING not found")
			}
			assert.Equal(t, tt.expected, *got)
		})
	}
}
