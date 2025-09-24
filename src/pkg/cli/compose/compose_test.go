package compose

import (
	"bytes"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/term"
)

func TestLoadProjectName(t *testing.T) {
	var tests = map[string]string{
		"noprojname":     "../../../testdata/noprojname/compose.yaml",
		"tests":          "../../../testdata/testproj/compose.yaml",
		"fancy-proj_dir": "../../../testdata/Fancy-Proj_Dir/compose.yaml",
		"altcomp":        "../../../testdata/alttestproj/altcomp.yaml",
	}

	for expectedName, path := range tests {
		t.Run("Load project name from compose file or directory:"+expectedName, func(t *testing.T) {
			loader := NewLoader(WithPath(path))
			name, err := loader.LoadProjectName(t.Context())
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
		loader := NewLoader(WithPath("../../../testdata/testproj/compose.yaml"))
		name, err := loader.LoadProjectName(t.Context())
		if err != nil {
			t.Fatalf("LoadProjectName() failed: %v", err)
		}

		if name != "overridename" {
			t.Errorf("LoadProjectName() failed: expected project name to be overwritten by env var, got %q", name)
		}
	})

	t.Run("--project-name has precedence over COMPOSE_PROJECT_NAME env var", func(t *testing.T) {
		t.Setenv("COMPOSE_PROJECT_NAME", "ignoreme")
		loader := NewLoader(WithProjectName("expectedname"))
		name, err := loader.LoadProjectName(t.Context())
		if err != nil {
			t.Fatalf("LoadProjectName() failed: %v", err)
		}

		if name != "expectedname" {
			t.Errorf("LoadProjectName() failed: expected project name to be overwritten by env var, got %q", name)
		}
	})
}

func TestLoadProjectNameWithoutComposeFile(t *testing.T) {
	loader := NewLoader(WithProjectName("testproj"))
	name, err := loader.LoadProjectName(t.Context())
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
		loader := NewLoader(WithPath("../../../testdata/noprojname/compose.yaml"))
		p, err := loader.LoadProject(t.Context())
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
		loader := NewLoader(WithPath("../../../testdata/Fancy-Proj_Dir/compose.yaml"))
		p, err := loader.LoadProject(t.Context())
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
		loader := NewLoader(WithPath("../../../testdata/testproj/compose.yaml"))
		p, err := loader.LoadProject(t.Context())
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
		loader := NewLoader(WithPath("../../../testdata/testproj/compose.yaml"))
		p, err := loader.LoadProject(t.Context())
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

	t.Run("use project name should not be overridden by tenantName", func(t *testing.T) {
		loader := NewLoader(WithPath("../../../testdata/testproj/compose.yaml"))
		p, err := loader.LoadProject(t.Context())
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
		t.Chdir("../../../testdata/alttestproj/subdir/subdir2")

		// execute test
		loader := NewLoader()
		p, err := loader.LoadProject(t.Context())
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
		loader := NewLoader(WithPath("../../../testdata/alttestproj/altcomp.yaml"))
		p, err := loader.LoadProject(t.Context())
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
	term.DefaultTerm = term.NewTerm(os.Stdin, &warnings, &warnings)

	loader := NewLoader(WithPath("../../../testdata/compose-go-warn/compose.yaml"))
	_, err := loader.LoadProject(t.Context())
	if err != nil {
		t.Fatalf("LoadProject() failed: %v", err)
	}

	if bytes.Count(warnings.Bytes(), []byte(`"yes" for boolean is not supported by YAML 1.2`)) != 1 {
		t.Errorf("Warning for using 'yes' for boolean from compose-go should appear exactly once")
	}
}

func TestComposeOnlyOneFile(t *testing.T) {
	t.Chdir("../../../testdata/toomany")

	loader := NewLoader()
	project, err := loader.LoadProject(t.Context())
	if err != nil {
		t.Fatalf("LoadProject() failed: %v", err)
	}

	if len(project.ComposeFiles) != 1 {
		t.Errorf("LoadProject() failed: expected only one config file, got %d", len(project.ComposeFiles))
	}
}

func TestComposeMultipleFiles(t *testing.T) {
	t.Chdir("../../../testdata/multiple")

	loader := NewLoader(WithPath("compose1.yaml", "compose2.yaml"))
	project, err := loader.LoadProject(t.Context())
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
