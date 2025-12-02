package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFixupConfigError(t *testing.T) {
	cfgErr := errors.New("missing configs: DB_PASSWORD")
	newErr := FixupConfigError(cfgErr)
	assert.EqualError(t, newErr, "The operation failed due to missing configs not being set, use the Defang tool called set_config to set the variable: missing configs: DB_PASSWORD")

	otherErr := errors.New("another error")
	res2 := FixupConfigError(otherErr)
	assert.EqualError(t, res2, otherErr.Error())
}
