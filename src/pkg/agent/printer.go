package agent

import (
	"fmt"
	"io"
)

type printer struct {
	outStream io.Writer
}

type Printer interface {
	Printf(format string, args ...interface{})
	Println(args ...interface{})
}

func (p printer) Printf(format string, args ...interface{}) {
	fmt.Fprintf(p.outStream, format, args...)
}

func (p printer) Println(args ...interface{}) {
	fmt.Fprintln(p.outStream, args...)
}
