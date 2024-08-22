package command

import (
	"testing"

	compose "github.com/compose-spec/compose-go/v2/types"
	assert "github.com/stretchr/testify/assert"
)

func TestIsOnlyManagedResources(t *testing.T) {

	t.Run("no services", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)

		assert.Equal(t, true, IsOnlyManagedResources(project))
	})

	t.Run("one service all managed", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)
		serviceCfg := compose.ServiceConfig{}
		extensions := compose.Extensions{"x-defang-redis": true}

		serviceCfg.Extensions = compose.Extensions(extensions)
		project.Services["service1"] = serviceCfg

		assert.Equal(t, true, IsOnlyManagedResources(project))
	})

	t.Run("one service unmanaged", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)
		serviceCfg := compose.ServiceConfig{}
		extensions := compose.Extensions{}
		serviceCfg.Extensions = compose.Extensions(extensions)
		project.Services["service1"] = serviceCfg

		assert.Equal(t, false, IsOnlyManagedResources(project))
	})

	t.Run("one service unmanaged, one service managed", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)
		project.Services["service1"] = compose.ServiceConfig{}

		serviceCfg := compose.ServiceConfig{}
		extensions := compose.Extensions{"x-defang-redis": true}
		serviceCfg.Extensions = compose.Extensions(extensions)
		project.Services["service2"] = serviceCfg

		assert.Equal(t, false, IsOnlyManagedResources(project))
	})

	t.Run("two service two unmanaged", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)
		project.Services["service1"] = compose.ServiceConfig{}

		serviceCfg := compose.ServiceConfig{}
		project.Services["service2"] = serviceCfg

		assert.Equal(t, false, IsOnlyManagedResources(project))
	})

	t.Run("one service two managed", func(t *testing.T) {
		project := &compose.Project{}
		project.Services = make(map[string]compose.ServiceConfig)
		serviceCfg := compose.ServiceConfig{}
		extensions := compose.Extensions{"x-defang-redis": true, "x-defang-postgres": true}
		serviceCfg.Extensions = compose.Extensions(extensions)
		project.Services["service1"] = serviceCfg

		assert.Equal(t, true, IsOnlyManagedResources(project))
	})

}
