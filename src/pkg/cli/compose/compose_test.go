package compose

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/term"
)

func TestLoadProjectName(t *testing.T) {
	var tests = map[string]string{
		"noprojname":     "../../../tests/noprojname/compose.yaml",
		"tests":          "../../../tests/testproj/compose.yaml",
		"fancy-proj_dir": "../../../tests/Fancy-Proj_Dir/compose.yaml",
		"altcomp":        "../../../tests/alttestproj/altcomp.yaml",
	}

	for expectedName, path := range tests {
		t.Run("Load project name from compose file or directory:"+expectedName, func(t *testing.T) {
			loader := NewLoaderWithPath(path)
			name, err := loader.LoadProjectName(context.Background())
			if err != nil {
				t.Fatalf("LoadProjectName() failed: %v", err)
			}
			if name != expectedName {
				t.Errorf("LoadProjectName() failed: expected project name %q, got %q", expectedName, name)
			}
		})
	}

	t.Run("COMPOSE_PROJECT_NAME env var should override project name", func(t *testing.T) {
		t.Setenv("COMPOSE_PROJECT_NAME", "overridename")
		loader := NewLoaderWithPath("../../../tests/testproj/compose.yaml")
		name, err := loader.LoadProjectName(context.Background())
		if err != nil {
			t.Fatalf("LoadProjectName() failed: %v", err)
		}

		if name != "overridename" {
			t.Errorf("LoadProjectName() failed: expected project name to be overwritten by env var, got %q", name)
		}
	})

	t.Run("--project-name has precidence over COMPOSE_PROJECT_NAME env var", func(t *testing.T) {
		t.Setenv("COMPOSE_PROJECT_NAME", "ignoreme")
		options := LoaderOptions{ProjectName: "expectedname"}
		loader := NewLoaderWithOptions(options)
		name, err := loader.LoadProjectName(context.Background())
		if err != nil {
			t.Fatalf("LoadProjectName() failed: %v", err)
		}

		if name != "expectedname" {
			t.Errorf("LoadProjectName() failed: expected project name to be overwritten by env var, got %q", name)
		}
	})

}

func TestLoadProjectNameWithoutComposeFile(t *testing.T) {
	loader := NewLoaderWithOptions(LoaderOptions{ProjectName: "testproj"})
	name, err := loader.LoadProjectName(context.Background())
	if err != nil {
		t.Fatalf("LoadProjectName() failed: %v", err)
	}

	if name != "testproj" {
		t.Errorf("LoadProjectName() failed: expected project name testproj, got %q", name)
	}
}

func TestLoadProject(t *testing.T) {
	term.SetDebug(testing.Verbose())

	t.Run("no project name defaults to parent directory name", func(t *testing.T) {
		loader := NewLoaderWithPath("../../../tests/noprojname/compose.yaml")
		p, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatalf("LoadProject() failed: %v", err)
		}
		if p.Name != "noprojname" { // Use the parent directory name as project name
			t.Errorf("LoadProject() failed: expected project name tenant-id, got %q", p.Name)
		}
		// the echo service has the donotstart profile so it is not included
		if len(p.Services) != 0 {
			t.Errorf("LoadProject() failed: expected 0 services, got %d", len(p.Services))
		}
	})

	t.Run("no project name defaults to fancy parent directory name", func(t *testing.T) {
		loader := NewLoaderWithPath("../../../tests/Fancy-Proj_Dir/compose.yaml")
		p, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatalf("LoadProject() failed: %v", err)
		}
		if p.Name != "fancy-proj_dir" { // Use the parent directory name as project name
			t.Errorf("LoadProject() failed: expected project name tenant-id, got %q", p.Name)
		}
		// the echo service has the donotstart profile so it is not included
		if len(p.Services) != 0 {
			t.Errorf("LoadProject() failed: expected 0 services, got %d", len(p.Services))
		}
	})

	t.Run("use project name in compose file", func(t *testing.T) {
		loader := NewLoaderWithPath("../../../tests/testproj/compose.yaml")
		p, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatalf("LoadProject() failed: %v", err)
		}
		if p.Name != "tests" {
			t.Errorf("LoadProject() failed: expected project name, got %q", p.Name)
		}
		if len(p.Services) != 1 {
			t.Errorf("LoadProject() failed: expected 1 services, got %d", len(p.Services))
		}
	})

	t.Run("COMPOSE_PROJECT_NAME env var should override project name", func(t *testing.T) {
		t.Setenv("COMPOSE_PROJECT_NAME", "overridename")
		loader := NewLoaderWithPath("../../../tests/testproj/compose.yaml")
		p, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatalf("LoadProject() failed: %v", err)
		}
		if p.Name != "overridename" {
			t.Errorf("LoadProject() failed: expected project name to be overwritten by env var, got %q", p.Name)
		}
		if len(p.Services) != 1 {
			t.Errorf("LoadProject() failed: expected 1 services, got %d", len(p.Services))
		}
	})

	t.Run("use project name should not be overriden by tenantID", func(t *testing.T) {
		loader := NewLoaderWithPath("../../../tests/testproj/compose.yaml")
		p, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatalf("LoadProject() failed: %v", err)
		}
		if p.Name != "tests" {
			t.Errorf("LoadProject() failed: expected project name tests, got %q", p.Name)
		}
		if len(p.Services) != 1 {
			t.Errorf("LoadProject() failed: expected 1 services, got %d", len(p.Services))
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
		p, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatalf("LoadProject() failed: %v", err)
		}
		if p.Name != "tests" {
			t.Errorf("LoadProject() failed: expected project name tests, got %q", p.Name)
		}
		if len(p.Services) != 1 {
			t.Errorf("LoadProject() failed: expected 1 services, got %d", len(p.Services))
		}
	})

	t.Run("load alternative compose file", func(t *testing.T) {
		loader := NewLoaderWithPath("../../../tests/alttestproj/altcomp.yaml")
		p, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatalf("LoadProject() failed: %v", err)
		}
		if p.Name != "altcomp" {
			t.Errorf("LoadProject() failed: expected project name altcomp, got %q", p.Name)
		}
		if len(p.Services) != 1 {
			t.Errorf("LoadProject() failed: expected 1 services, got %d", len(p.Services))
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
	_, err := loader.LoadProject(context.Background())
	if err != nil {
		t.Fatalf("LoadProject() failed: %v", err)
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
	project, err := loader.LoadProject(context.Background())
	if err != nil {
		t.Errorf("LoadProject() failed: %v", err)
	}

	if len(project.ComposeFiles) != 1 {
		t.Errorf("LoadProject() failed: expected only one config file, got %d", len(project.ComposeFiles))
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
	project, err := loader.LoadProject(context.Background())
	if err != nil {
		t.Fatalf("LoadProject() failed: %v", err)
	}

	if len(project.ComposeFiles) != 2 {
		t.Errorf("LoadProject() failed: expected 2 compose files, got %d", len(project.ComposeFiles))
	}

	if len(project.Services) != 2 {
		t.Errorf("LoadProject() failed: expected 2 services, got %d", len(project.Services))
	}
}
