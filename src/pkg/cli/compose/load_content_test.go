package compose

import (
	"context"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	testRunCompose(t, func(t *testing.T, path string) {
		loader := NewLoader(WithPath(path))
		p, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		content, err := p.MarshalYAML()
		if err != nil {
			t.Fatal(err)
		}
		rt, err := LoadFromContent(context.Background(), content)
		if err != nil {
			t.Fatal(err)
		}
		if p.Name != rt.Name {
			t.Errorf("expected project name %q, got %q", p.Name, rt.Name)
		}
	})
}
