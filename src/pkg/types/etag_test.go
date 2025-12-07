package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewEtag(t *testing.T) {
	etag := NewEtag()
	_, err := ParseEtag(etag)
	assert.NoError(t, err, "NewEtag() produced invalid etag: %s", etag)
}

func TestParseEtag(t *testing.T) {
	validEtag := "abc123def456"
	etag, err := ParseEtag(validEtag)
	assert.NoError(t, err)
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
		assert.Error(t, err)
	}
}
