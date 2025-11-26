package tools

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/DefangLabs/defang/src/pkg/term"
)

func CaptureTerm(f func() (any, error)) (string, error) {
	return captureTerm(false, f)
}

func TeeTerm(f func() (any, error)) (string, error) {
	return captureTerm(true, f)
}

// TODO: consider using generics or string instead of any
func captureTerm(tee bool, f func() (any, error)) (string, error) {
	// replace the default term with a new term that writes to a buffer
	originalTerm := term.DefaultTerm
	outBuffer := bytes.NewBuffer(nil)
	errBuffer := bytes.NewBuffer(nil)
	var outWriter io.Writer
	var errWriter io.Writer
	if tee {
		outWriter = io.MultiWriter(outBuffer, os.Stdout)
		errWriter = io.MultiWriter(errBuffer, os.Stderr)
		// Replace newTerm creation below to use outWriter and errWriter
	} else {
		outWriter = outBuffer
		errWriter = errBuffer
	}
	newTerm := term.NewTerm(
		os.Stdin,
		// whenever something is written to outBuffer or errBuffer, also write it to the original term's outStream
		outWriter,
		errWriter,
	)
	term.DefaultTerm = newTerm
	defer func() {
		term.DefaultTerm = originalTerm
	}()
	result, err := f()
	output := outBuffer.String() + errBuffer.String()
	return output + fmt.Sprint(result), err
}
