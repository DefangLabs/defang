package term

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/muesli/termenv"
	"golang.org/x/term"
)

type Term struct {
	stdin          FileReader
	stdout, stderr io.Writer
	out, err       *termenv.Output
	hadWarnings    bool
	debug          bool

	isTerminal bool
	hasDarkBg  bool
}

var DefaultTerm = NewTerm(os.Stdin, os.Stdout, os.Stderr)

type Color = termenv.ANSIColor

const (
	BrightCyan    = termenv.ANSIBrightCyan
	InfoColor     = termenv.ANSIBrightMagenta
	ErrorColor    = termenv.ANSIBrightRed
	WarnColor     = termenv.ANSIYellow      // not bright to improve readability on light backgrounds
	DebugColor    = termenv.ANSIBrightBlack // Gray
	ResetColorStr = termenv.CSI + termenv.ResetSeq + "m"
)

type FileReader interface {
	io.Reader
	Fd() uintptr
}

type FileWriter interface {
	io.Writer
	Fd() uintptr
}

func NewTerm(stdin FileReader, stdout, stderr io.Writer) *Term {
	t := &Term{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		out:    termenv.NewOutput(stdout),
		err:    termenv.NewOutput(stderr),
	}
	t.hasDarkBg = t.out.HasDarkBackground()
	if hasTermInEnv() {
		if fout, ok := stdout.(interface{ Fd() uintptr }); ok {
			t.isTerminal = term.IsTerminal(int(fout.Fd())) && term.IsTerminal(int(stdin.Fd()))
		}
	}
	return t
}

func (t Term) Stdio() (FileReader, termenv.File, io.Writer) {
	return t.stdin, t.out.TTY(), t.err
}

func (t *Term) ForceColor(color bool) {
	if color {
		t.out = termenv.NewOutput(t.stdout, termenv.WithProfile(termenv.ANSI))
		t.err = termenv.NewOutput(t.stderr, termenv.WithProfile(termenv.ANSI))
	} else {
		t.out = termenv.NewOutput(t.stdout, termenv.WithProfile(termenv.Ascii))
		t.err = termenv.NewOutput(t.stderr, termenv.WithProfile(termenv.Ascii))
	}
}

func (t *Term) SetDebug(debug bool) {
	t.debug = debug
}

func (t *Term) DoDebug() bool {
	return t.debug
}

func (t *Term) HasDarkBackground() bool {
	return t.hasDarkBg
}

func (t *Term) IsTerminal() bool {
	return t.isTerminal
}

func (t *Term) HadWarnings() bool {
	return t.hadWarnings
}

func (t *Term) SetHadWarnings(had bool) {
	t.hadWarnings = had
}

func (t *Term) StdoutCanColor() bool {
	return doColor(t.out)
}

func (t *Term) StderrCanColor() bool {
	return doColor(t.err)
}

func (t *Term) HideCursor() {
	t.out.HideCursor()
}

func (t *Term) ShowCursor() {
	t.out.ShowCursor()
}

func (t *Term) ClearLine() {
	t.out.ClearLine()
}

func (t *Term) Reset() {
	t.out.Reset()
}

// DoColor returns true if the provided output's profile is not Ascii.
func doColor(o *termenv.Output) bool {
	return o.Profile != termenv.Ascii
}

func output(w *termenv.Output, c Color, msg string) (int, error) {
	if len(msg) == 0 {
		return 0, nil
	}
	var buf strings.Builder
	if doColor(w) {
		push(&buf, doColor(w), c, msg)
		msg = buf.String()
	}
	return w.WriteString(msg)
}

func push(w io.Writer, canColor bool, c Color, v ...any) (l int, e error) {
	if canColor {
		n, err := io.WriteString(w, termenv.CSI+c.Sequence(false)+"m")
		l += n
		if err != nil {
			return l, err
		}
		defer func() {
			n, err := io.WriteString(w, ResetColorStr)
			l += n
			e = err
		}()
	}

	n, err := fmt.Fprint(w, v...)
	l += n
	if err != nil {
		return l, err
	}
	return l, nil
}

func ensureNewline(s string) string {
	if len(s) == 0 || (s[len(s)-1] != '\n' && s[len(s)-1] != '\r') {
		return s + "\n"
	}
	return s
}

func ensurePrefix(s string, prefix string) string {
	// Don't add prefix to empty strings or strings that already have it
	if len(s) == 0 || strings.HasPrefix(s, prefix) {
		return s
	}
	return prefix + s
}

func (t *Term) Printc(c Color, v ...any) (int, error) {
	return output(t.out, c, fmt.Sprint(v...))
}

func (t *Term) Printlnc(c Color, v ...any) (int, error) {
	return output(t.out, c, ensureNewline(fmt.Sprintln(v...)))
}

func (t *Term) Printfc(c Color, format string, v ...any) (int, error) {
	line := ensureNewline(fmt.Sprintf(format, v...))
	return output(t.out, c, line)
}

func (t *Term) Print(v ...any) (int, error) {
	return fmt.Fprint(t.out, v...)
}

func (t *Term) Println(v ...any) (int, error) {
	return fmt.Fprint(t.out, ensureNewline(fmt.Sprintln(v...)))
}

func (t *Term) Printf(format string, v ...any) (int, error) {
	return fmt.Fprint(t.out, ensureNewline(fmt.Sprintf(format, v...)))
}

func (t *Term) Debug(v ...any) (int, error) {
	if !t.debug {
		return 0, nil
	}
	return output(t.out, DebugColor, ensurePrefix(fmt.Sprintln(v...), " - "))
}

func (t *Term) Debugf(format string, v ...any) (int, error) {
	if !t.debug {
		return 0, nil
	}
	return output(t.out, DebugColor, ensureNewline(ensurePrefix(fmt.Sprintf(format, v...), " - ")))
}

func (t *Term) Info(v ...any) (int, error) {
	return output(t.out, InfoColor, ensurePrefix(fmt.Sprintln(v...), " * "))
}

func (t *Term) Infof(format string, v ...any) (int, error) {
	return output(t.out, InfoColor, ensureNewline(ensurePrefix(fmt.Sprintf(format, v...), " * ")))
}

func (t *Term) Warn(v ...any) (int, error) {
	t.hadWarnings = true
	return output(t.out, WarnColor, ensurePrefix(fmt.Sprintln(v...), " ! "))
}

func (t *Term) Warnf(format string, v ...any) (int, error) {
	t.hadWarnings = true
	return output(t.out, WarnColor, ensureNewline(ensurePrefix(fmt.Sprintf(format, v...), " ! ")))
}

func (t *Term) Error(v ...any) (int, error) {
	return output(t.err, ErrorColor, fmt.Sprintln(v...))
}

func (t *Term) Errorf(format string, v ...any) (int, error) {
	line := ensureNewline(fmt.Sprintf(format, v...))
	return output(t.err, ErrorColor, line)
}

func (t *Term) Fatal(msg any) {
	Error("Error:", msg)
	os.Exit(1)
}

func (t *Term) Fatalf(format string, v ...any) {
	Errorf("Error: "+format, v...)
	os.Exit(1)
}

func Print(v ...any) (int, error) {
	return DefaultTerm.Print(v...)
}

func Println(v ...any) (int, error) {
	return DefaultTerm.Println(v...)
}

func Printf(format string, v ...any) (int, error) {
	return DefaultTerm.Printf(format, v...)
}

func Printc(c Color, v ...any) (int, error) {
	return DefaultTerm.Printc(c, v...)
}

func Printlnc(c Color, v ...any) (int, error) {
	return DefaultTerm.Printlnc(c, v...)
}

func Printfc(c Color, format string, v ...any) (int, error) {
	return DefaultTerm.Printfc(c, format, v...)
}

func Debug(v ...any) (int, error) {
	return DefaultTerm.Debug(v...)
}

func Debugf(format string, v ...any) (int, error) {
	return DefaultTerm.Debugf(format, v...)
}

func Info(v ...any) (int, error) {
	return DefaultTerm.Info(v...)
}

func Infof(format string, v ...any) (int, error) {
	return DefaultTerm.Infof(format, v...)
}

func Warn(v ...any) (int, error) {
	return DefaultTerm.Warn(v...)
}

func Warnf(format string, v ...any) (int, error) {
	return DefaultTerm.Warnf(format, v...)
}

func Error(v ...any) (int, error) {
	return DefaultTerm.Error(v...)
}

func Errorf(format string, v ...any) (int, error) {
	return DefaultTerm.Errorf(format, v...)
}

func Fatal(msg any) {
	DefaultTerm.Fatal(msg)
}

func Fatalf(format string, v ...any) {
	DefaultTerm.Fatalf(format, v...)
}

func ForceColor(color bool) {
	DefaultTerm.ForceColor(color)
}

func SetDebug(debug bool) {
	DefaultTerm.SetDebug(debug)
}

func DoDebug() bool {
	return DefaultTerm.DoDebug()
}

func HasDarkBackground() bool {
	return DefaultTerm.HasDarkBackground()
}

func IsTerminal() bool {
	return DefaultTerm.IsTerminal()
}

func HadWarnings() bool {
	return DefaultTerm.HadWarnings()
}

func SetHadWarnings(had bool) {
	DefaultTerm.SetHadWarnings(had)
}

func StdoutCanColor() bool {
	return DefaultTerm.StdoutCanColor()
}

func StderrCanColor() bool {
	return DefaultTerm.StderrCanColor()
}

func HideCursor() {
	DefaultTerm.HideCursor()
}

func ShowCursor() {
	DefaultTerm.ShowCursor()
}

func ClearLine() {
	DefaultTerm.ClearLine()
}

func Reset() {
	DefaultTerm.Reset()
}

/* ANSI escape codes https://en.wikipedia.org/wiki/ANSI_escape_code
 * Fp/Fe/Fs: ESC [0-WYZ\`-~] 						 				(0x30-0x7E except 'X', '[', ']', '^', '_')
 * CSI:      ESC '[' [0-?]* [ -/]* [@-~]  							(common commands like color, cursor movement, etc.)
 * OSC:      ESC ('X' | ']' | '^' | '_') .*? (BEL | ESC '\' | $)	(commands that set window title, etc.)
 */
var ansiRegex = regexp.MustCompile("\x1b(?:[@-WYZ\\\\`-~]|\\[[0-?]*[ -/]*[@-~]|[X\\]^_].*?(?:\x1b\\\\|\x07|$))")

func StripAnsi(s string) string {
	return ansiRegex.ReplaceAllLiteralString(s, "")
}

type MessageBuilder struct {
	strings.Builder

	canColor bool
}

func NewMessageBuilder(canColor bool) *MessageBuilder {
	return &MessageBuilder{canColor: canColor}
}

func (b *MessageBuilder) Printc(c Color, v ...any) (int, error) {
	return push(&b.Builder, b.canColor, c, v...)
}
