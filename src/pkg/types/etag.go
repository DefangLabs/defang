package types

import (
	"errors"

	"github.com/DefangLabs/defang/src/pkg"
)

type ETag = string

func NewEtag() ETag {
	return pkg.RandomID()
}

func ParseEtag(s string) (ETag, error) {
	if len(s) != 12 {
		return "", errors.New("invalid etag: must be 12 characters long")
	}
	if !isBase36(s) {
		return "", errors.New("invalid etag: must be base36 (0-9, a-z)")
	}
	return s, nil
}

func isBase36(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'z') {
			return false
		}
	}
	return true
}
