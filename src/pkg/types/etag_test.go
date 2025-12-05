package types

import "testing"

func TestNewEtag(t *testing.T) {
	etag := NewEtag()
	if len(etag) != 12 {
		t.Errorf("NewEtag() length = %d, want 12", len(etag))
	}
	if !isBase36(etag) {
		t.Errorf("NewEtag() = %s, want base36 string", etag)
	}
}

func TestParseEtag(t *testing.T) {
	validEtag := "abc123def456"
	etag, err := ParseEtag(validEtag)
	if err != nil {
		t.Errorf("ParseEtag(%s) returned error: %v", validEtag, err)
	}
	if etag != validEtag {
		t.Errorf("ParseEtag(%s) = %s, want %s", validEtag, etag, validEtag)
	}

	invalidEtags := []string{
		"short",
		"thisiswaytoolongetagvalue",
		"invalid$$chars",
		"UPPERCASE12",
	}

	for _, invalid := range invalidEtags {
		_, err := ParseEtag(invalid)
		if err == nil {
			t.Errorf("ParseEtag(%s) = nil error, want error", invalid)
		}
	}
}
