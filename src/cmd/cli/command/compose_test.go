package command

import (
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestGetUnreferencedManagedResources(t *testing.T) {

	t.Run("no services", func(t *testing.T) {
		project := []*defangv1.ServiceInfo{}
		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resources, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("one service all managed", func(t *testing.T) {
		project := []*defangv1.ServiceInfo{}
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service1", Postgres: &defangv1.Postgres{}}})

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("one service unmanaged", func(t *testing.T) {
		project := []*defangv1.ServiceInfo{}
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service1"}})

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resource, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("one service unmanaged, one service managed", func(t *testing.T) {
		project := []*defangv1.ServiceInfo{}
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service1"}})
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service2", Postgres: &defangv1.Postgres{}}})

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("two service two unmanaged", func(t *testing.T) {
		project := []*defangv1.ServiceInfo{}
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service1"}})
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service2"}})

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resource, got %d (%v)", len(managed), managed)
		}
	})

	t.Run("one service two managed", func(t *testing.T) {
		project := []*defangv1.ServiceInfo{}
		project = append(project, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: "service1", Postgres: &defangv1.Postgres{}, Redis: &defangv1.Redis{}}})

		managed := GetUnreferencedManagedResources(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%s)", len(managed), managed)
		}
	})
}
