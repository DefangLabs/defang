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
	testAllConfigResolutionFiles(t, func(t *testing.T, name, path string) {
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
			defangConfigs = []string{"SECRET_KEY", "API_TOKEN"}
		case "mixed-sources":
			defangConfigs = []string{"SECRET_KEY"}
		case "interpolated-values":
			defangConfigs = []string{"DB_USER", "DB_PASSWORD", "API_TOKEN"}
		case "multiple-services":
			defangConfigs = []string{"REDIS_PASSWORD", "DATABASE_URL"}
		default:
			defangConfigs = []string{}
		}

		err = PrintConfigResolutionSummary(proj, defangConfigs)
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

func testAllConfigResolutionFiles(t *testing.T, f func(t *testing.T, name, path string)) {
	t.Helper()

	composeRegex := regexp.MustCompile(`^(?i)(docker-)?compose.ya?ml$`)
	err := filepath.WalkDir("testdata/configresolution", func(path string, d os.DirEntry, err error) error {
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
