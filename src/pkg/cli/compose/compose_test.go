package compose

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/term"
)

func TestLoadCompose(t *testing.T) {
	term.SetDebug(testing.Verbose())

	t.Run("no project name defaults to parent directory name", func(t *testing.T) {
		loader := NewLoaderWithPath("../../../tests/noprojname/compose.yaml")
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "noprojname" { // Use the parent directory name as project name
			t.Errorf("LoadCompose() failed: expected project name tenant-id, got %q", p.Name)
		}
	})

	t.Run("no project name defaults to fancy parent directory name", func(t *testing.T) {
		loader := NewLoaderWithPath("../../../tests/Fancy-Proj_Dir/compose.yaml")
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "fancy-proj_dir" { // Use the parent directory name as project name
			t.Errorf("LoadCompose() failed: expected project name tenant-id, got %q", p.Name)
		}
	})

	t.Run("use project name in compose file", func(t *testing.T) {
		loader := NewLoaderWithPath("../../../tests/testproj/compose.yaml")
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "tests" {
			t.Errorf("LoadCompose() failed: expected project name, got %q", p.Name)
		}
	})

	t.Run("COMPOSE_PROJECT_NAME env var should override project name", func(t *testing.T) {
		t.Setenv("COMPOSE_PROJECT_NAME", "overridename")
		loader := NewLoaderWithPath("../../../tests/testproj/compose.yaml")
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "overridename" {
			t.Errorf("LoadCompose() failed: expected project name to be overwritten by env var, got %q", p.Name)
		}
	})

	t.Run("use project name should not be overriden by tenantID", func(t *testing.T) {
		loader := NewLoaderWithPath("../../../tests/testproj/compose.yaml")
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "tests" {
			t.Errorf("LoadCompose() failed: expected project name tests, got %q", p.Name)
		}
	})

	t.Run("load starting from a sub directory", func(t *testing.T) {
		cwd, _ := os.Getwd()

		// setup
		setup := func() {
			os.MkdirAll("../../../tests/alttestproj/subdir/subdir2", 0755)
			os.Chdir("../../../tests/alttestproj/subdir/subdir2")
		}

		//teardown
		teardown := func() {
			os.Chdir(cwd)
			os.RemoveAll("../../../tests/alttestproj/subdir")
		}

		setup()
		t.Cleanup(teardown)

		// execute test
		loader := NewLoaderWithPath("")
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "tests" {
			t.Errorf("LoadCompose() failed: expected project name tests, got %q", p.Name)
		}
	})

	t.Run("load alternative compose file", func(t *testing.T) {
		loader := NewLoaderWithPath("../../../tests/alttestproj/altcomp.yaml")
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "altcomp" {
			t.Errorf("LoadCompose() failed: expected project name altcomp, got %q", p.Name)
		}
	})
}

func TestComposeGoNoDoubleWarningLog(t *testing.T) {

	oldTerm := term.DefaultTerm
	t.Cleanup(func() {
		term.DefaultTerm = oldTerm
	})

	var warnings bytes.Buffer
	term.DefaultTerm = term.NewTerm(&warnings, &warnings)

	loader := NewLoaderWithPath("../../../tests/compose-go-warn/compose.yaml")
	_, err := loader.LoadCompose(context.Background())
	if err != nil {
		t.Fatalf("LoadCompose() failed: %v", err)
	}

	if bytes.Count(warnings.Bytes(), []byte(`"yes" for boolean is not supported by YAML 1.2`)) != 1 {
		t.Errorf("Warning for using 'yes' for boolean from compose-go should appear exactly once")
	}
}

func TestComposeOnlyOneFile(t *testing.T) {
	cwd, _ := os.Getwd()
	t.Cleanup(func() {
		os.Chdir(cwd)
	})
	os.Chdir("../../../tests/toomany")

	loader := NewLoaderWithPath("")
	project, err := loader.LoadCompose(context.Background())
	if err != nil {
		t.Errorf("LoadCompose() failed: %v", err)
	}

	if len(project.ComposeFiles) != 1 {
		t.Errorf("LoadCompose() failed: expected only one config file, got %d", len(project.ComposeFiles))
	}
}

func TestComposeMultipleFiles(t *testing.T) {
	cwd, _ := os.Getwd()
	t.Cleanup(func() {
		os.Chdir(cwd)
	})
	os.Chdir("../../../tests/multiple")

	composeFiles := []string{"compose1.yaml", "compose2.yaml"}
	loader := NewLoaderWithOptions(LoaderOptions{ConfigPaths: composeFiles})
	project, err := loader.LoadCompose(context.Background())
	if err != nil {
		t.Fatalf("LoadCompose() failed: %v", err)
	}

	if len(project.ComposeFiles) != 2 {
		t.Errorf("LoadCompose() failed: expected 2 compose files, got %d", len(project.ComposeFiles))
	}

	if len(project.Services) != 2 {
		t.Errorf("LoadCompose() failed: expected 2 services, got %d", len(project.Services))
	}
}
