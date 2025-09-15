package term

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/muesli/termenv"
	"golang.org/x/term"
)

type Term struct {
	stdin          FileReader
	stdout, stderr io.Writer
	out, err       *termenv.Output
	debug          bool
	jsonMode       bool

	isTerminal bool
	hasDarkBg  bool

	warnings []string
}

var DefaultTerm = NewTerm(os.Stdin, os.Stdout, os.Stderr)

type Color = termenv.ANSIColor

const (
	BrightCyan = termenv.ANSIBrightCyan
	InfoColor  = termenv.ANSIBrightMagenta
	ErrorColor = termenv.ANSIBrightRed
	WarnColor  = termenv.ANSIYellow      // not bright to improve readability on light backgrounds
	DebugColor = termenv.ANSIBrightBlack // Gray

	boldColorStr  = termenv.CSI + termenv.BoldSeq + "m"
	resetColorStr = termenv.CSI + termenv.ResetSeq + "m"
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

func (t *Term) SetJSONMode(jsonMode bool) {
	t.jsonMode = jsonMode
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
	return len(t.warnings) > 0
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

// doColor returns true if the provided output's profile is not Ascii.
func doColor(o *termenv.Output) bool {
	return o.Profile != termenv.Ascii
}

func output(w *termenv.Output, c Color, msg string) (int, error) {
	if len(msg) == 0 {
		return 0, nil
	}
	var buf strings.Builder
	if doColor(w) {
		fprintc(&buf, true, c, msg)
		msg = buf.String() // this uses the buffer to avoid allocation, so make sure buf is not garbage collected
	}
	return w.WriteString(msg)
}

func fprintc(w io.Writer, canColor bool, c Color, v ...any) (l int, e error) {
	if canColor {
		n, err := io.WriteString(w, termenv.CSI+c.Sequence(false)+"m")
		l += n
		if err != nil {
			return l, err
		}
		defer func() {
			n, err := io.WriteString(w, resetColorStr)
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
	if t.jsonMode {
		return t.outputJSON("debug", t.out, v...)
	}
	return output(t.out, DebugColor, ensurePrefix(fmt.Sprintln(v...), " - "))
}

func (t *Term) Debugf(format string, v ...any) (int, error) {
	if !t.debug {
		return 0, nil
	}
	if t.jsonMode {
		return t.outputJSON("debug", t.out, fmt.Sprintf(format, v...))
	}
	return output(t.out, DebugColor, ensureNewline(ensurePrefix(fmt.Sprintf(format, v...), " - ")))
}

func (t *Term) Info(v ...any) (int, error) {
	if t.jsonMode {
		return t.outputJSON("info", t.out, v...)
	}
	return output(t.out, InfoColor, ensurePrefix(fmt.Sprintln(v...), " * "))
}

func (t *Term) Infof(format string, v ...any) (int, error) {
	if t.jsonMode {
		return t.outputJSON("info", t.out, fmt.Sprintf(format, v...))
	}
	return output(t.out, InfoColor, ensureNewline(ensurePrefix(fmt.Sprintf(format, v...), " * ")))
}

func (t *Term) Warn(v ...any) (int, error) {
	if t.jsonMode {
		msg := strings.TrimSpace(fmt.Sprintln(v...))
		t.warnings = append(t.warnings, msg)
		return t.outputJSON("warn", t.out, msg)
	}
	msg := ensurePrefix(fmt.Sprintln(v...), " ! ")
	t.warnings = append(t.warnings, msg)
	return output(t.out, WarnColor, msg)
}

func (t *Term) Warnf(format string, v ...any) (int, error) {
	if t.jsonMode {
		msg := strings.TrimSpace(fmt.Sprintf(format, v...))
		t.warnings = append(t.warnings, msg)
		return t.outputJSON("warn", t.out, msg)
	}
	msg := ensureNewline(ensurePrefix(fmt.Sprintf(format, v...), " ! "))
	t.warnings = append(t.warnings, msg)
	return output(t.out, WarnColor, msg)
}

func (t *Term) Error(v ...any) (int, error) {
	if t.jsonMode {
		return t.outputJSON("error", t.err, v...)
	}
	return output(t.err, ErrorColor, fmt.Sprintln(v...))
}

func (t *Term) Errorf(format string, v ...any) (int, error) {
	if t.jsonMode {
		return t.outputJSON("error", t.err, fmt.Sprintf(format, v...))
	}
	line := ensureNewline(fmt.Sprintf(format, v...))
	return output(t.err, ErrorColor, line)
}

// Deprecated: use proper error handling instead
func (t *Term) Fatal(msg any) {
	Error("Error:", msg)
	os.Exit(1)
}

// Deprecated: use proper error handling instead
func (t *Term) Fatalf(format string, v ...any) {
	Errorf("Error: "+format, v...)
	os.Exit(1)
}

func (t *Term) getAllWarnings() []string {
	slices.Sort(t.warnings)
	return slices.Compact(t.warnings)
}

func (t *Term) FlushWarnings() (int, error) {
	uniqueWarnings := t.getAllWarnings()
	t.ResetWarnings()
	bytesWritten := 0

	// unique warnings only
	for _, w := range uniqueWarnings {
		bytes, err := output(t.out, WarnColor, w)
		bytesWritten += bytes
		if err != nil {
			return bytesWritten, err
		}
	}

	return bytesWritten, nil
}

func (t *Term) ResetWarnings() {
	t.warnings = nil
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

// Deprecated: use proper error handling instead
func Fatal(msg any) {
	DefaultTerm.Fatal(msg)
}

// Deprecated: use proper error handling instead
func Fatalf(format string, v ...any) {
	DefaultTerm.Fatalf(format, v...)
}

func FlushWarnings() (int, error) {
	return DefaultTerm.FlushWarnings()
}

func ResetWarnings() {
	DefaultTerm.ResetWarnings()
}

func ForceColor(color bool) {
	DefaultTerm.ForceColor(color)
}

func SetDebug(debug bool) {
	DefaultTerm.SetDebug(debug)
}

func SetJSONMode(jsonMode bool) {
	DefaultTerm.SetJSONMode(jsonMode)
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

type LogEntry struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

func (t *Term) outputJSON(level string, w *termenv.Output, v ...any) (int, error) {
	entry := LogEntry{
		Level:   level,
		Message: strings.TrimSpace(fmt.Sprint(v...)),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return 0, err
	}
	return fmt.Fprint(w, string(data)+"\n")
}

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
	return fprintc(&b.Builder, b.canColor, c, v...)
}
