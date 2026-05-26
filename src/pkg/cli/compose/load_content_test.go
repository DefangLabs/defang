package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundTrip(t *testing.T) {
	testAllComposeFiles(t, func(t *testing.T, name, path string) {
		loader := NewLoader(WithPath(path))
		p, err := loader.LoadProject(t.Context())
		if strings.HasPrefix(name, "invalid-") {
			assert.Error(t, err, "Expected error for invalid compose file: %s", path)
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := MarshalYAML(p)
		if err != nil {
			t.Fatal(err)
		}
		rt, err := LoadFromContent(t.Context(), content, "**invalid name**")
		if err != nil {
			t.Fatal(err)
		}
		if p.Name != rt.Name {
			t.Errorf("expected project name %q, got %q", p.Name, rt.Name)
		}
	})
}

func TestLoadFromContent(t *testing.T) {
	tdt := []struct {
		desc        string
		compose     string
		fallback    string
		wantProject string
	}{
		{
			desc:        "compose with project name",
			compose:     "name: project1\nservices:\n  service1:\n    image: nginx",
			fallback:    "",
			wantProject: "project1",
		},
		{
			desc:        "compose without project name",
			compose:     "services:\n  service1:\n    image: nginx",
			fallback:    "project2",
			wantProject: "project2",
		},
		{
			desc:        "compose with project name and ignored fallback",
			compose:     "name: project3\nservices:\n  service1:\n    image: nginx",
			fallback:    "project4",
			wantProject: "project3",
		},
		{
			desc:        "compose with unknown network",
			compose:     "name: project4\nservices:\n  service1:\n    image: nginx\n    networks:\n      - unknown",
			wantProject: "project4",
		},
		{
			desc:        "compose with cpus",
			compose:     "name: project5\nservices:\n  service1:\n    image: nginx\n    deploy:\n      resources:\n        reservations:\n          cpus: 2",
			wantProject: "project5",
		},
		{
			desc:        "compose with config",
			compose:     "name: project6\nservices:\n  service1:\n    image: nginx\n    environment:\n      - CONFIG",
			wantProject: "project6",
		},
		{
			desc:        "compose with interpolation",
			compose:     "name: project7\nservices:\n  service1:\n    image: nginx\n    environment:\n      - ENVVAR=${ENVVAR}",
			wantProject: "project7",
		},
		{
			desc:        "should not load env_file",
			compose:     "name: project9\nservices:\n  service1:\n    image: nginx\n    env_file:\n      - asdf",
			wantProject: "project9",
		},
	}

	t.Setenv("ENVVAR", "value")

	for _, tt := range tdt {
		t.Run(tt.desc, func(t *testing.T) {
			project, err := LoadFromContent(t.Context(), []byte(tt.compose), tt.fallback)
			if err != nil {
				t.Fatal(err)
			}
			if project.Name != tt.wantProject {
				t.Errorf("Expected project name to be project1, got: %s", project.Name)
			}
			if len(project.Services) != 1 {
				t.Errorf("Expected 1 service, got %d", len(project.Services))
			}
			if _, ok := project.Services["service1"]; !ok {
				t.Errorf("Expected service1 to be present, got %v", project.Services)
			}
			if project.Services["service1"].Image != "nginx" {
				t.Errorf("Expected service1 image to be nginx, got: %s", project.Services["service1"].Image)
			}
			if config, ok := project.Services["service1"].Environment["CONFIG"]; ok {
				if config != nil {
					t.Errorf("Expected CONFIG to be nil, got %q", *config)
				}
			}
			if envvar, ok := project.Services["service1"].Environment["ENVVAR"]; ok {
				if *envvar != "${ENVVAR}" {
					t.Errorf("Expected ENVVAR to be ${ENVVAR}, got %q", *envvar)
				}
			}
		})
	}
}

func TestLoadProjectWithStackEnvFile(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "compose.yaml")
	stackEnvPath := filepath.Join(dir, ".env.mystack")

	require.NoError(t, os.WriteFile(composePath, []byte(`name: envfiles
services:
  app:
    image: ${IMAGE}
    environment:
      SHARED: ${SHARED}
      STACK_VALUE: ${STACK_VALUE}
      UNRESOLVED: ${UNRESOLVED}
      INTERPOLATED: ${INTERPOLATED}
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("IMAGE=nginx\nSHARED=base\nBASE=basevalue\n"), 0o600))
	require.NoError(t, os.WriteFile(stackEnvPath, []byte("SHARED=stack\nSTACK_VALUE=fromstack\nINTERPOLATED=${BASE}-stack\nUNUSED=unused\n"), 0o600))

	project, err := NewLoader(WithPath(composePath), WithStackName("mystack")).LoadProject(t.Context())
	require.NoError(t, err)

	service := project.Services["app"]
	assert.Equal(t, "nginx", service.Image)
	assert.Equal(t, "stack", *service.Environment["SHARED"])
	assert.Equal(t, "fromstack", *service.Environment["STACK_VALUE"])
	assert.Equal(t, "${UNRESOLVED}", *service.Environment["UNRESOLVED"])
	assert.Equal(t, "basevalue-stack", *service.Environment["INTERPOLATED"])
}

func TestLoadProjectWithOnlyStackEnvFile(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "compose.yaml")
	stackEnvPath := filepath.Join(dir, ".env.mystack")

	require.NoError(t, os.WriteFile(composePath, []byte(`name: envfiles
services:
  app:
    image: ${IMAGE}
`), 0o600))
	require.NoError(t, os.WriteFile(stackEnvPath, []byte("IMAGE=busybox\nUNUSED=unused\n"), 0o600))

	project, err := NewLoader(WithPath(composePath), WithStackName("mystack")).LoadProject(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "busybox", project.Services["app"].Image)
}

func TestLoadProjectWithStackEnvFileFromDiscoveredProjectDir(t *testing.T) {
	dir := t.TempDir()
	childDir := filepath.Join(dir, "child")
	require.NoError(t, os.Mkdir(childDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(`name: envfiles
services:
  app:
    image: ${IMAGE}
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("IMAGE=base\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.mystack"), []byte("IMAGE=stack\n"), 0o600))

	t.Chdir(childDir)
	project, err := NewLoader(WithStackName("mystack")).LoadProject(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "stack", project.Services["app"].Image)
}

func TestLoadProjectWithEmptyStackEnvFile(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "compose.yaml")
	stackEnvPath := filepath.Join(dir, ".env.mystack")

	require.NoError(t, os.WriteFile(composePath, []byte(`name: envfiles
services:
  app:
    image: ${IMAGE}
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("IMAGE=nginx\n"), 0o600))
	require.NoError(t, os.WriteFile(stackEnvPath, nil, 0o600))

	project, err := NewLoader(WithPath(composePath), WithStackName("mystack")).LoadProject(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "nginx", project.Services["app"].Image)
}

func TestLoadProjectWithStackEnvDirectory(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "compose.yaml")
	require.NoError(t, os.WriteFile(composePath, []byte(`name: envfiles
services:
  app:
    image: nginx
`), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".env.mystack"), 0o700))

	_, err := NewLoader(WithPath(composePath), WithStackName("mystack")).LoadProject(t.Context())
	require.ErrorContains(t, err, "is a directory")
}

func TestLoadProjectWithEnvStackFixture(t *testing.T) {
	project, err := NewLoader(
		WithPath("testdata/envstackfixture/compose.yaml"),
		WithStackName("teststackname"),
	).LoadProject(t.Context())
	require.NoError(t, err)

	service := project.Services["app"]
	assert.Equal(t, "nginx", service.Image)
	assert.Equal(t, "stack", *service.Environment["SHARED"])
	assert.Equal(t, "fromstack", *service.Environment["STACK_ONLY"])
	assert.Equal(t, "${UNRESOLVED}", *service.Environment["UNRESOLVED"])
}
