package clouds

import (
	"strings"

	"github.com/defang-io/defang/src/pkg"
)

const (
	CdTaskPrefix = "defang-cd" // WARNING: renaming this practically deletes the Pulumi state
	DefangPrefix = "Defang"    // prefix for all resources created by Defang
)

var (
	// Changing this will cause issues if two clients with different versions are using the same account
	CdImage = pkg.Getenv("DEFANG_CD_IMAGE", "public.ecr.aws/defang-io/cd:public-beta")
)

type Warning interface {
	Error() string
	Warning() string
}

func (w Warnings) Error() string {
	var buf strings.Builder
	for _, warning := range w {
		buf.WriteString(warning.Warning())
		buf.WriteByte('\n')
	}
	return buf.String()
}

// Deprecated: replace with proper logging
type WarningError string

func (w WarningError) Error() string {
	return string(w)
}

func (w WarningError) Warning() string {
	return string(w)
}

type Warnings []Warning
