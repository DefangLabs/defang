package tools

import (
	"bytes"
	"io"
	"os"

	"github.com/DefangLabs/defang/src/pkg/term"
)

func CaptureTerm(f func() (string, error)) (string, error) {
	// replace the default term with a new term that writes to a buffer
	originalTerm := term.DefaultTerm
	outBuffer := bytes.NewBuffer(nil)
	errBuffer := bytes.NewBuffer(nil)
	newTerm := term.NewTerm(
		os.Stdin,
		// whenever something is written to outBuffer or errBuffer, also write it to the original term's outStream
		io.MultiWriter(outBuffer, os.Stdout),
		io.MultiWriter(errBuffer, os.Stderr),
	)
	term.DefaultTerm = newTerm
	defer func() {
		term.DefaultTerm = originalTerm
	}()
	result, err := f()
	output := outBuffer.String() + errBuffer.String()
	return output + result, err
}
