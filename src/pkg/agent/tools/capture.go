package tools

import (
	"bytes"
	"os"

	"github.com/DefangLabs/defang/src/pkg/term"
)

func CaptureTerm(f func() (string, error)) (string, error) {
	// replace the default term with a new term that writes to a buffer
	originalTerm := term.DefaultTerm
	outStream := bytes.NewBuffer(nil)
	errStream := bytes.NewBuffer(nil)
	newTerm := term.NewTerm(
		os.Stdin,
		outStream,
		errStream,
	)
	term.DefaultTerm = newTerm
	defer func() {
		term.DefaultTerm = originalTerm
	}()
	result, err := f()
	output := outStream.String() + errStream.String()
	return output + result, err
}
