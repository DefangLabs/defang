package compose

import (
	"testing"
)

func TestRoundTrip(t *testing.T) {
	testRunCompose(t, func(t *testing.T, path string) {
		loader := NewLoader(WithPath(path))
		p, err := loader.LoadProject(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		content, err := p.MarshalYAML()
		if err != nil {
			t.Fatal(err)
		}
		rt, err := LoadFromContent(t.Context(), content, "should-not-be-used")
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
				t.Errorf("Expected project name to be project1, got %s", project.Name)
			}
			if len(project.Services) != 1 {
				t.Errorf("Expected 1 service, got %d", len(project.Services))
			}
			if _, ok := project.Services["service1"]; !ok {
				t.Errorf("Expected service1 to be present, got %v", project.Services)
			}
			if project.Services["service1"].Image != "nginx" {
				t.Errorf("Expected service1 image to be nginx, got %s", project.Services["service1"].Image)
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
