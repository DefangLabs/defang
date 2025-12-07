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
		return "", errors.New("invalid deployment etag: must be 12 characters long")
	}
	if !pkg.IsValidRandomID(s) {
		return "", errors.New("invalid deployment etag: must be base-36")
	}
	return s, nil
}
