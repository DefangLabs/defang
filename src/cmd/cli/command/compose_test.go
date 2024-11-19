package command

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestInitializeTailCmd(t *testing.T) {
	t.Run("", func(t *testing.T) {
		for _, cmd := range RootCmd.Commands() {
			if cmd.Use == "logs" {
				cmd.Execute()
				return
			}
		}
	})
}

func TestGetUnreferencedManagedResources(t *testing.T) {
	t.Run("no services", func(t *testing.T) {
		project := types.Services{}

		managed, unmanaged := splitManagedAndUnmanagedServices(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resources, got %d (%v)", len(managed), managed)
		}

		if len(unmanaged) != 0 {
			t.Errorf("Expected 0 unmanaged resources, got %d (%v)", len(unmanaged), unmanaged)
		}
	})

	t.Run("one service all managed", func(t *testing.T) {
		project := types.Services{
			"service1": types.ServiceConfig{
				Extensions: map[string]any{"x-defang-postgres": true},
			},
		}

		managed, unmanaged := splitManagedAndUnmanagedServices(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%v)", len(managed), managed)
		}

		if len(unmanaged) != 0 {
			t.Errorf("Expected 0 unmanaged resource, got %d (%v)", len(unmanaged), unmanaged)
		}
	})

	t.Run("one service unmanaged", func(t *testing.T) {
		project := types.Services{
			"service1": types.ServiceConfig{},
		}

		managed, unmanaged := splitManagedAndUnmanagedServices(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resource, got %d (%v)", len(managed), managed)
		}

		if len(unmanaged) != 1 {
			t.Errorf("Expected 1 unmanaged resource, got %d (%v)", len(unmanaged), unmanaged)
		}
	})

	t.Run("one service unmanaged, one service managed", func(t *testing.T) {
		project := types.Services{
			"service1": types.ServiceConfig{},
			"service2": types.ServiceConfig{
				Extensions: map[string]any{"x-defang-postgres": true},
			},
		}

		managed, unmanaged := splitManagedAndUnmanagedServices(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%v)", len(managed), managed)
		}

		if len(unmanaged) != 1 {
			t.Errorf("Expected 1 unmanaged resource, got %d (%v)", len(unmanaged), unmanaged)
		}
	})

	t.Run("two service two unmanaged", func(t *testing.T) {
		project := types.Services{
			"service1": types.ServiceConfig{},
			"service2": types.ServiceConfig{},
		}

		managed, unmanaged := splitManagedAndUnmanagedServices(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resource, got %d (%v)", len(managed), managed)
		}

		if len(unmanaged) != 2 {
			t.Errorf("Expected 2 unmanaged resource, got %d (%v)", len(unmanaged), unmanaged)
		}
	})

	t.Run("one service two managed", func(t *testing.T) {
		project := types.Services{
			"service1": types.ServiceConfig{},
			"service2": types.ServiceConfig{
				Extensions: map[string]any{"x-defang-postgres": true},
			},
			"service3": types.ServiceConfig{
				Extensions: map[string]any{"x-defang-redis": true},
			},
		}

		managed, unmanaged := splitManagedAndUnmanagedServices(project)
		if len(managed) != 2 {
			t.Errorf("Expected 2 managed resource, got %d (%s)", len(managed), managed)
		}
		if len(unmanaged) != 1 {
			t.Errorf("Expected 1 unmanaged resource, got %d (%s)", len(unmanaged), unmanaged)
		}
	})
}
