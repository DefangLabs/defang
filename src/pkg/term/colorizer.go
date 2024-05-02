package term

import (
	"fmt"
	"os"

	"github.com/muesli/termenv"
	"golang.org/x/term"
)

var (
	IsTerminal  = term.IsTerminal(int(os.Stdout.Fd())) && term.IsTerminal(int(os.Stdin.Fd())) && isTerminal()
	Stdout      = termenv.NewOutput(os.Stdout)
	Stderr      = termenv.NewOutput(os.Stderr)
	CanColor    = doColor(Stdout)
	CanColorErr = doColor(Stderr)
	DoDebug     bool
	HadWarnings bool
)

type Color = termenv.ANSIColor

const (
	BrightCyan = termenv.ANSIBrightCyan
	InfoColor  = termenv.ANSIBrightMagenta
	ErrorColor = termenv.ANSIBrightRed
	WarnColor  = termenv.ANSIBrightYellow
	DebugColor = termenv.ANSIBrightBlack // Gray
)

// doColor returns true if the provided output's profile is not Ascii.
func doColor(o *termenv.Output) bool {
	return o.Profile != termenv.Ascii
}

func ForceColor(color bool) {
	if color {
		Stdout = termenv.NewOutput(os.Stdout, termenv.WithProfile(termenv.ANSI))
		Stderr = termenv.NewOutput(os.Stderr, termenv.WithProfile(termenv.ANSI))
	} else {
		Stdout = termenv.NewOutput(os.Stdout, termenv.WithProfile(termenv.Ascii))
		Stderr = termenv.NewOutput(os.Stderr, termenv.WithProfile(termenv.Ascii))
	}
}

func output(w *termenv.Output, c Color, msg string) (int, error) {
	if len(msg) == 0 {
		return 0, nil
	}
	if doColor(w) {
		w.WriteString(termenv.CSI + c.Sequence(false) + "m")
		defer w.Reset()
	}
	return w.WriteString(msg)
}

func outputf(w *termenv.Output, c Color, format string, v ...any) (int, error) {
	line := fmt.Sprintf(format, v...)
	if len(line) == 0 || (line[len(line)-1] != '\n' && line[len(line)-1] != '\r') {
		line += "\n" // add newline, like log.Printf
	}
	return output(w, c, line)
}

func Fprint(w *termenv.Output, c Color, v ...any) (int, error) {
	return output(w, c, fmt.Sprint(v...))
}

func Fprintln(w *termenv.Output, c Color, v ...any) (int, error) {
	return output(w, c, fmt.Sprintln(v...))
}

func Fprintf(w *termenv.Output, c Color, format string, v ...any) (int, error) {
	return output(w, c, fmt.Sprintf(format, v...))
}

func Print(c Color, v ...any) (int, error) {
	return Fprint(Stdout, c, v...)
}

func Println(c Color, v ...any) (int, error) {
	return Fprintln(Stdout, c, v...)
}

func Printf(c Color, format string, v ...any) (int, error) {
	return Fprintf(Stdout, c, format, v...)
}

func Debug(v ...any) (int, error) {
	if !DoDebug {
		return 0, nil
	}
	return Fprintln(Stderr, DebugColor, v...)
}

func Debugf(format string, v ...any) (int, error) {
	if !DoDebug {
		return 0, nil
	}
	return outputf(Stderr, DebugColor, format, v...)
}

func Info(v ...any) (int, error) {
	return Fprintln(Stdout, InfoColor, v...)
}

func Infof(format string, v ...any) (int, error) {
	return outputf(Stdout, InfoColor, format, v...)
}

func Warn(v ...any) (int, error) {
	HadWarnings = true
	return Fprintln(Stderr, WarnColor, v...)
}

func Warnf(format string, v ...any) (int, error) {
	HadWarnings = true
	return outputf(Stderr, WarnColor, format, v...)
}

func Error(v ...any) (int, error) {
	return Fprintln(Stderr, ErrorColor, v...)
}

func Errorf(format string, v ...any) (int, error) {
	return outputf(Stderr, ErrorColor, format, v...)
}

func Fatal(msg any) {
	Error("Error:", msg)
	os.Exit(1)
}

func Fatalf(format string, v ...any) {
	Errorf("Error: "+format, v...)
	os.Exit(1)
}
