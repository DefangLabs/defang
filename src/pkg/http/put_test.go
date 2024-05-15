package http

import "testing"

func TestRemoveQueryParam(t *testing.T) {
	url := "https://example.com/foo?bar=baz"
	expected := "https://example.com/foo"
	actual := RemoveQueryParam(url)
	if actual != expected {
		t.Errorf("expected %q, got %q", expected, actual)
	}
}
