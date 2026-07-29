package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func TestPrintConfigResolutionSummary(t *testing.T) {
	testAllConfigResolutionFiles(t, "testdata/config-resolution", func(t *testing.T, name, path string) {
		stdout, _ := term.SetupTestTerm(t)

		loader := compose.NewLoader(compose.WithPath(path))
		proj, err := loader.LoadProject(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		// Determine which config variables should be treated as defang configs based on the test case
		var defangConfigs []string
		switch name {
		case "defang-config-only":
			defangConfigs = []string{"SECRET_KEY", "API_TOKEN", "DB_USER"}
		case "mixed-sources":
			defangConfigs = []string{"SECRET_KEY"}
		case "interpolated-values":
			defangConfigs = []string{"DB_USER", "DB_PASSWORD", "API_TOKEN"}
		case "multiple-services":
			defangConfigs = []string{"REDIS_PASSWORD", "DATABASE_URL"}
		default:
			defangConfigs = []string{}
		}

		err = printConfigResolutionSummary(proj, defangConfigs, false)
		if err != nil {
			t.Fatalf("PrintConfigResolutionSummary() error = %v", err)
		}

		output := stdout.Bytes()

		// Compare the output with the golden file
		if err := pkg.Compare(output, path+".golden"); err != nil {
			t.Error(err)
		}
	})
}

func TestPrintRedactedConfigResolutionSummary(t *testing.T) {
	testAllConfigResolutionFiles(t, "testdata/redact-config", func(t *testing.T, name, path string) {
		stdout, _ := term.SetupTestTerm(t)

		loader := compose.NewLoader(compose.WithPath(path))
		proj, err := loader.LoadProject(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		err = printConfigResolutionSummary(proj, nil, true)
		if err != nil {
			t.Fatalf("PrintConfigResolutionSummary() error = %v", err)
		}

		output := stdout.Bytes()

		// Compare the output with the golden file
		if err := pkg.Compare(output, path+".golden"); err != nil {
			t.Error(err)
		}
	})
}

func TestDetermineConfigSource(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	// DEFANG_STACK is deliberately also listed as a defang config to prove the
	// reserved-name check takes precedence over the config check.
	defangConfigs := map[string]struct{}{"SECRET_KEY": {}, "DEFANG_STACK": {}}

	tests := []struct {
		name       string
		envKey     string
		envValue   *string
		wantSource Source
		wantValue  string
	}{
		{"reserved provider", "DEFANG_PROVIDER", strPtr("aws"), SourceDefang, "aws"},
		{"reserved project name", "COMPOSE_PROJECT_NAME", strPtr("myproj"), SourceDefang, "myproj"},
		{"reserved wins over defang config", "DEFANG_STACK", strPtr("mystack"), SourceDefang, "mystack"},
		{"reserved with nil value", "DEFANG_PROVIDER", nil, SourceDefang, ""},
		{"defang config is masked", "SECRET_KEY", strPtr("supersecret"), SourceDefangConfig, configMaskedValue},
		{"nil non-reserved is unset config", "API_TOKEN", nil, SourceDefangConfig, ""},
		{"interpolated value", "DB_URL", strPtr("postgres://${DB_USER}@db"), SourceInterpolation, "postgres://${DB_USER}@db"},
		{"plain compose value", "PORT", strPtr("8080"), SourceComposeFile, "8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSource, gotValue := determineConfigSource(tt.envKey, tt.envValue, defangConfigs)
			if gotSource != tt.wantSource {
				t.Errorf("source = %q, want %q", gotSource, tt.wantSource)
			}
			if gotValue != tt.wantValue {
				t.Errorf("value = %q, want %q", gotValue, tt.wantValue)
			}
		})
	}
}

func testAllConfigResolutionFiles(t *testing.T, dir string, f func(t *testing.T, name, path string)) {
	t.Helper()

	composeRegex := regexp.MustCompile(`^(?i)(docker-)?compose.ya?ml$`)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !composeRegex.MatchString(d.Name()) {
			return err
		}

		t.Run(path, func(t *testing.T) {
			t.Log(path)
			f(t, filepath.Base(filepath.Dir(path)), path)
		})
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}
