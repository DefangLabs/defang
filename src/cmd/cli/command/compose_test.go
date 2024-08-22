package command

import (
	"testing"

	compose "github.com/compose-spec/compose-go/v2/types"
)

func TestGetUnreferencedManagedResources(t *testing.T) {

	t.Run("no services", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resources, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("one service all managed", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)
		serviceCfg := compose.ServiceConfig{Name: "service1"}
		extensions := compose.Extensions{"x-defang-redis": true}

		serviceCfg.Extensions = compose.Extensions(extensions)
		project.Services[serviceCfg.Name] = serviceCfg

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("one service unmanaged", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)
		serviceCfg := compose.ServiceConfig{Name: "service1"}
		extensions := compose.Extensions{}
		serviceCfg.Extensions = compose.Extensions(extensions)
		project.Services[serviceCfg.Name] = serviceCfg

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resource, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("one service unmanaged, one service managed", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)
		serviceCfg := compose.ServiceConfig{Name: "service1"}
		project.Services[serviceCfg.Name] = serviceCfg

		extensions := compose.Extensions{"x-defang-redis": true}
		serviceCfg2 := compose.ServiceConfig{Name: "service2"}
		serviceCfg2.Extensions = compose.Extensions(extensions)
		project.Services[serviceCfg2.Name] = serviceCfg2

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("two service two unmanaged", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)
		serviceCfg := compose.ServiceConfig{Name: "service1"}
		project.Services[serviceCfg.Name] = serviceCfg

		serviceCfg2 := compose.ServiceConfig{Name: "service2"}
		project.Services[serviceCfg2.Name] = serviceCfg2

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resource, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("one service two managed", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)
		serviceCfg := compose.ServiceConfig{Name: "service1"}
		extensions := compose.Extensions{"x-defang-redis": true, "x-defang-postgres": true}
		serviceCfg.Extensions = compose.Extensions(extensions)
		project.Services[serviceCfg.Name] = serviceCfg

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%s)", len(managed), managed)
		}
	})

	t.Run("one service depends on a second with managed resource", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)
		serviceCfg := compose.ServiceConfig{Name: "service1"}
		extensions := compose.Extensions{"x-defang-redis": true}
		serviceCfg.Extensions = compose.Extensions(extensions)
		project.Services[serviceCfg.Name] = serviceCfg

		serviceCfg2 := compose.ServiceConfig{Name: "service2"}
		serviceCfg2.DependsOn = compose.DependsOnConfig{serviceCfg.Name: compose.ServiceDependency{}}
		project.Services[serviceCfg2.Name] = serviceCfg2

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 0 {
			t.Errorf("Expected 1 managed resource, got %d (%s)", len(managed), managed)
		}
	})

	t.Run("one service depends on a second service", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)
		serviceCfg := compose.ServiceConfig{Name: "service1"}
		extensions := compose.Extensions{"x-defang-redis": true}
		serviceCfg.Extensions = compose.Extensions(extensions)
		project.Services[serviceCfg.Name] = serviceCfg

		serviceCfg2 := compose.ServiceConfig{Name: "service2"}
		serviceCfg2.DependsOn = compose.DependsOnConfig{"service3": compose.ServiceDependency{}}
		project.Services[serviceCfg2.Name] = serviceCfg2

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%s)", len(managed), managed)
		}
	})
}
