package types

import (
	"errors"
	"strconv"

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
	_, err := strconv.ParseUint(s, 36, 64)
	if err != nil {
		return "", errors.New("invalid etag: must be base36")
	}
	return s, nil
}
