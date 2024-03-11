package cli

import (
	"fmt"
	"os"

	"github.com/muesli/termenv"
	"golang.org/x/term"
)

var (
	IsTerminal = term.IsTerminal(int(os.Stdout.Fd())) && term.IsTerminal(int(os.Stdin.Fd())) && os.Getenv("TERM") != ""
	stdout     = termenv.NewOutput(os.Stdout)
	stderr     = termenv.NewOutput(os.Stderr)
	CanColor   = doColor(stdout)
)

type Color = termenv.ANSIColor

const (
	Nop        Color = -1
	BrightCyan       = termenv.ANSIBrightCyan
	InfoColor        = termenv.ANSIBrightMagenta
	ErrorColor       = termenv.ANSIBrightRed
	WarnColor        = termenv.ANSIBrightYellow
	DebugColor       = termenv.ANSIBrightBlack // Gray
)

// doColor returns true if the provided output's profile is not Ascii.
func doColor(o *termenv.Output) bool {
	return o.Profile != termenv.Ascii
}

func ForceColor(color bool) {
	if color {
		stdout = termenv.NewOutput(os.Stdout, termenv.WithProfile(termenv.ANSI))
		stderr = termenv.NewOutput(os.Stderr, termenv.WithProfile(termenv.ANSI))
	} else {
		stdout = termenv.NewOutput(os.Stdout, termenv.WithProfile(termenv.Ascii))
		stderr = termenv.NewOutput(os.Stderr, termenv.WithProfile(termenv.Ascii))
	}
}

func Fprint(w *termenv.Output, c Color, v ...any) (int, error) {
	if doColor(w) && c != Nop {
		w.WriteString(termenv.CSI + c.Sequence(false) + "m")
		defer w.Reset()
	}
	return fmt.Fprint(w, v...)
}

func Fprintln(w *termenv.Output, c Color, v ...any) (int, error) {
	if doColor(w) && c != Nop {
		w.WriteString(termenv.CSI + c.Sequence(false) + "m")
		defer w.Reset()
	}
	return fmt.Fprintln(w, v...)
}

func Print(c Color, v ...any) (int, error) {
	return Fprint(stdout, c, v...)
}

func Println(c Color, v ...any) (int, error) {
	return Fprintln(stdout, c, v...)
}

func Debug(v ...any) (int, error) {
	if !DoDebug {
		return 0, nil
	}
	return Fprintln(stderr, DebugColor, v...)
}

func Info(v ...any) (int, error) {
	return Println(InfoColor, v...)
}

func Warn(v ...any) (int, error) {
	HadWarnings = true
	return Fprintln(stderr, WarnColor, v...)
}

func Error(v ...any) (int, error) {
	return Fprintln(stderr, ErrorColor, v...)
}
