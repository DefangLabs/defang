package command

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

// Helper function to convert a string to a pointer to a string
func stringPtr(s string) *string {
	return &s
}

// Helper function to check if two slices contain the same elements, regardless of order
func sameElements(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	elemCount := make(map[string]int)
	for _, elem := range a {
		elemCount[elem]++
	}
	for _, elem := range b {
		if elemCount[elem] == 0 {
			return false
		}
		elemCount[elem]--
	}
	return true
}

// Test function for collectUnsetEnvVars
func TestCollectUnsetEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		project  *types.Project
		expected []string
	}{
		{
			name:     "Nil project",
			project:  nil,
			expected: []string{},
		},
		{
			name: "No unset environment variables (map structure)",
			project: &types.Project{
				Services: map[string]types.ServiceConfig{
					"service1": {
						Name: "service1",
						Environment: types.MappingWithEquals{
							"ENV1": stringPtr("value1"),
							"ENV2": stringPtr("value2"),
						},
					},
				},
			},
			expected: []string{},
		},
		{
			name: "Some unset environment variables (map structure)",
			project: &types.Project{
				Services: map[string]types.ServiceConfig{
					"service1": {
						Name: "service1",
						Environment: types.MappingWithEquals{
							"ENV1": stringPtr("value1"),
							"ENV2": nil,
							"ENV3": stringPtr(""),
						},
					},
				},
			},
			expected: []string{"ENV2"},
		},
		{
			name: "All unset environment variables (map structure)",
			project: &types.Project{
				Services: map[string]types.ServiceConfig{
					"service1": {
						Name: "service1",
						Environment: types.MappingWithEquals{
							"ENV1": nil,
							"ENV2": nil,
						},
					},
				},
			},
			expected: []string{"ENV1", "ENV2"},
		},
		{
			name: "Some unset environment variables (array structure)",
			project: &types.Project{
				Services: map[string]types.ServiceConfig{
					"service1": {
						Name: "service1",
						Environment: types.MappingWithEquals{
							"ENV1": stringPtr("value1"),
							"ENV2": nil,
						},
					},
				},
			},
			expected: []string{"ENV2"},
		},
		{
			name: "Multiple services with unset environment variables",
			project: &types.Project{
				Services: map[string]types.ServiceConfig{
					"service1": {
						Name: "service1",
						Environment: types.MappingWithEquals{
							"ENV1": stringPtr("value1"),
							"ENV2": nil,
						},
					},
					"service2": {
						Name: "service2",
						Environment: types.MappingWithEquals{
							"ENV3": stringPtr(""),
							"ENV4": stringPtr("value4"),
							"ENV5": nil,
						},
					},
				},
			},
			expected: []string{"ENV2", "ENV5"},
		},
		{
			name: "Services with map and array structure",
			project: &types.Project{
				Services: map[string]types.ServiceConfig{
					"x": {
						Name: "x",
						Environment: types.MappingWithEquals{
							"regular": stringPtr("value"),
							"empty":   stringPtr(""),
							"config":  nil,
						},
					},
					"y": {
						Name: "y",
						Environment: types.MappingWithEquals{
							"REDIS_HOST": stringPtr("x"),
							"EMPTY":      stringPtr(""),
							"CONFIG":     nil,
						},
					},
				},
			},
			expected: []string{"config", "CONFIG"},
		},
		{
			name: "Service with interpolated var",
			project: &types.Project{
				Services: map[string]types.ServiceConfig{
					"service1": {
						Name: "service1",
						Environment: types.MappingWithEquals{
							"ENV1": stringPtr("${ENV2}"),
						},
					},
				},
			},
			expected: []string{"ENV2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collectUnsetEnvVars(tt.project)
			if !sameElements(result, tt.expected) {
				t.Errorf("collectUnsetEnvVars() = %v, want %v", result, tt.expected)
			}
		})
	}
}
