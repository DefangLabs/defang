package dryrun

import "errors"

var (
	DoDryRun = false

	ErrDryRun = errors.New("dry run")
)
