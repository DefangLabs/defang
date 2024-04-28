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
		Stdout = termenv.NewOutput(os.Stdout, termenv.WithProfile(termenv.ANSI))
		Stderr = termenv.NewOutput(os.Stderr, termenv.WithProfile(termenv.ANSI))
	} else {
		Stdout = termenv.NewOutput(os.Stdout, termenv.WithProfile(termenv.Ascii))
		Stderr = termenv.NewOutput(os.Stderr, termenv.WithProfile(termenv.Ascii))
	}
}

func output(w *termenv.Output, c Color, msg string, addNewLine bool) (int, error) {
	if len(msg) == 0 {
		return 0, nil
	}
	if doColor(w) && c != Nop {
		w.WriteString(termenv.CSI + c.Sequence(false) + "m")
		defer w.Reset()
	}
	if addNewLine && msg[len(msg)-1] != '\n' && msg[len(msg)-1] != '\r' {
		msg += "\n"
	}
	return fmt.Fprint(w, msg)
}

func Fprint(w *termenv.Output, c Color, v ...any) (int, error) {
	return output(w, c, fmt.Sprint(v...), false)
}

func Fprintln(w *termenv.Output, c Color, v ...any) (int, error) {
	return output(w, c, fmt.Sprintln(v...), false)
}

func Fprintf(w *termenv.Output, c Color, format string, v ...any) (int, error) {
	return output(w, c, fmt.Sprintf(format, v...), false)
}

func Flog(w *termenv.Output, c Color, v ...any) (int, error) {
	return output(w, c, fmt.Sprintln(v...), true)
}

func Flogf(w *termenv.Output, c Color, format string, v ...any) (int, error) {
	return output(w, c, fmt.Sprintf(format, v...), true)
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
	return Flog(Stderr, DebugColor, v...)
}

func Debugf(format string, v ...any) (int, error) {
	if !DoDebug {
		return 0, nil
	}
	return Flogf(Stderr, DebugColor, format, v...)
}

func Info(v ...any) (int, error) {
	return Flog(Stdout, InfoColor, v...)
}

func Infof(format string, v ...any) (int, error) {
	return Flogf(Stdout, InfoColor, format, v...)
}

func Warn(v ...any) (int, error) {
	HadWarnings = true
	return Flog(Stderr, WarnColor, v...)
}

func Warnf(format string, v ...any) (int, error) {
	HadWarnings = true
	return Flogf(Stderr, WarnColor, format, v...)
}

func Error(v ...any) (int, error) {
	return Flog(Stderr, ErrorColor, v...)
}

func Errorf(format string, v ...any) (int, error) {
	return Flogf(Stderr, ErrorColor, format, v...)
}

func Fatal(msg any) {
	Error("Error:", msg)
	os.Exit(1)
}

func Fatalf(format string, v ...any) {
	Errorf("Error: "+format, v...)
	os.Exit(1)
}
