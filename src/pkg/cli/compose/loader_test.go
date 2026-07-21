package compose

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestInterpolationEnv(t *testing.T) {
	const composeYAML = `name: interpolationenvtest
services:
  web:
    image: alpine
    environment:
      - PROVIDER=${DEFANG_PROVIDER}
      - STACK=${DEFANG_STACK}
      - ACCESS_TOKEN=${DEFANG_ACCESS_TOKEN}
`
	dir := t.TempDir()
	composePath := filepath.Join(dir, "compose.yaml")
	require.NoError(t, os.WriteFile(composePath, []byte(composeYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("DEFANG_PROVIDER=dotenv\nDEFANG_STACK=dotenv\n"), 0o644))
	t.Setenv("DEFANG_ACCESS_TOKEN", "secret")

	loader := NewLoaderFromOptions(LoaderOptions{
		ConfigPaths: []string{composePath},
		InterpolationEnv: map[string]string{
			"DEFANG_PROVIDER": "aws",
			"DEFANG_STACK":    "production",
		},
	})
	project, err := loader.LoadProject(t.Context())
	require.NoError(t, err)

	env := project.Services["web"].Environment
	require.NotNil(t, env["PROVIDER"])
	require.NotNil(t, env["STACK"])
	require.NotNil(t, env["ACCESS_TOKEN"])
	assert.Equal(t, "aws", *env["PROVIDER"])
	assert.Equal(t, "production", *env["STACK"])
	assert.Equal(t, "${DEFANG_ACCESS_TOKEN}", *env["ACCESS_TOKEN"])
}

// TestDefaultEnvFiles covers the convention-based env files: when no env file
// is set explicitly (--env-file or COMPOSE_ENV_FILES), the loader layers the
// DefaultEnvFiles candidates (e.g. .env, .env.<provider>, .env.<stack>), later
// files overriding earlier ones. Missing files are skipped.
func TestDefaultEnvFiles(t *testing.T) {
	const composeYAML = `name: envdefaultstest
services:
  web:
    image: alpine
    environment:
      - BASE=${BASE}
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
	writeFile(".env", "BASE=from_dotenv\nGREETING=from_dotenv\nSOURCE=from_dotenv\n")
	writeFile(".env.aws", "GREETING=from_aws\nSOURCE=from_aws\n")
	writeFile(".env.mystack", "SOURCE=from_mystack\n")
	flagEnv := writeFile("flag.env", "GREETING=from_flag\n")

	// A sibling project without a base .env to verify the other default env files load on their own.
	noDotenvDir := t.TempDir()
	noDotenvCompose := filepath.Join(noDotenvDir, "compose.yaml")
	if err := os.WriteFile(noDotenvCompose, []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noDotenvDir, ".env.mystack"), []byte("SOURCE=from_mystack\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		composePath    string   // defaults to composePath
		envFiles       []string // explicit --env-file (LoaderOptions.EnvFiles)
		envVar         string   // COMPOSE_ENV_FILES; "" means unset
		defaultFiles   []string
		disableEnvFile bool // COMPOSE_DISABLE_ENV_FILE
		expected       map[string]string
	}{
		{
			name:         "default env files layer, most specific wins",
			defaultFiles: []string{".env", ".env.aws", ".env.mystack"},
			expected:     map[string]string{"BASE": "from_dotenv", "GREETING": "from_aws", "SOURCE": "from_mystack"},
		},
		{
			name:         "missing default env files are skipped",
			defaultFiles: []string{".env", ".env.gcp", ".env.mystack"},
			expected:     map[string]string{"BASE": "from_dotenv", "GREETING": "from_dotenv", "SOURCE": "from_mystack"},
		},
		{
			name:         "stack env file loads even without a base .env",
			composePath:  noDotenvCompose,
			defaultFiles: []string{".env", ".env.aws", ".env.mystack"},
			expected:     map[string]string{"BASE": "${BASE}", "GREETING": "${GREETING}", "SOURCE": "from_mystack"},
		},
		{
			name:         "explicit env-file overrides the convention",
			envFiles:     []string{flagEnv},
			defaultFiles: []string{".env", ".env.aws", ".env.mystack"},
			expected:     map[string]string{"BASE": "${BASE}", "GREETING": "from_flag", "SOURCE": "${SOURCE}"},
		},
		{
			name:         "COMPOSE_ENV_FILES overrides the convention",
			envVar:       flagEnv,
			defaultFiles: []string{".env", ".env.aws", ".env.mystack"},
			expected:     map[string]string{"BASE": "${BASE}", "GREETING": "from_flag", "SOURCE": "${SOURCE}"},
		},
		{
			name:           "COMPOSE_DISABLE_ENV_FILE disables the convention",
			defaultFiles:   []string{".env", ".env.aws", ".env.mystack"},
			disableEnvFile: true,
			expected:       map[string]string{"BASE": "${BASE}", "GREETING": "${GREETING}", "SOURCE": "${SOURCE}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVar != "" {
				t.Setenv(composeEnvFilesEnvVar, tt.envVar)
			} else {
				os.Unsetenv(composeEnvFilesEnvVar)
			}
			if tt.disableEnvFile {
				t.Setenv("COMPOSE_DISABLE_ENV_FILE", "1")
			}
			path := tt.composePath
			if path == "" {
				path = composePath
			}
			loader := NewLoaderFromOptions(LoaderOptions{
				ConfigPaths:     []string{path},
				EnvFiles:        tt.envFiles,
				DefaultEnvFiles: tt.defaultFiles,
			})
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

	t.Run("stat errors other than not-exist are returned", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("root ignores directory permissions")
		}
		unreadable := filepath.Join(t.TempDir(), "unreadable")
		if err := os.Mkdir(unreadable, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) }) // let TempDir cleanup succeed
		fn := withDefaultEnvFiles([]string{".env"})
		err := fn(&cli.ProjectOptions{WorkingDir: unreadable})
		assert.ErrorContains(t, err, "default env file")
	})

	t.Run("duplicate names are deduped", func(t *testing.T) {
		// e.g. a stack named after its provider; the shared file must be loaded once
		fn := withDefaultEnvFiles([]string{".env", ".env.aws", ".env.aws"})
		o := &cli.ProjectOptions{WorkingDir: dir}
		if err := fn(o); err != nil {
			t.Fatal(err)
		}
		expected := []string{filepath.Join(dir, ".env"), filepath.Join(dir, ".env.aws")}
		assert.Equal(t, expected, o.EnvFiles)
	})
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
