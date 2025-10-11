package term

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// DefaultBufferSize is the default number of recent messages to keep in the circular buffer
var DefaultBufferSize = 10

type circularBuffer struct {
	size         int
	linesWritten int
	index        int
	data         []string
}

func (c *circularBuffer) write(msg string) {
	c.linesWritten++
	if len(c.data) < c.size {
		c.data = append(c.data, msg)
	} else {
		c.data[c.index] = msg
	}
	c.index = (c.index + 1) % c.size
}

func (c *circularBuffer) read() []string {
	if c.linesWritten <= c.size {
		return slices.Clone(c.data)
	}

	messages := make([]string, 0, c.size)
	startIdx := c.index

	// Collect messages in chronological order
	for i := range c.size {
		idx := (startIdx + i) % c.size
		messages = append(messages, c.data[idx])
	}
	return messages
}

func NewCircularBuffer(bufferSize int) circularBuffer {
	if bufferSize <= 0 {
		bufferSize = 1 // ensure at least 1 element
	}
	return circularBuffer{
		size:         bufferSize,
		linesWritten: 0,
		index:        0,
		data:         make([]string, 0, bufferSize),
	}
}

type Term struct {
	stdin          FileReader
	stdout, stderr io.Writer
	out, err       *termenv.Output
	debug          bool

	isTerminal bool
	hasDarkBg  bool

	warnings []string
	buffer   circularBuffer
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
		buffer: NewCircularBuffer(DefaultBufferSize),
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

// GetAllMessages returns all messages currently stored in the buffer in chronological order
func (t Term) GetAllMessages() []string {
	return t.buffer.read()
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

func (t *Term) output(c Color, v ...any) (int, error) {
	msg := fmt.Sprint(v...)
	t.buffer.write(msg)
	return output(t.out, c, msg)
}

func (t *Term) Printc(c Color, v ...any) (int, error) {
	return t.output(c, v...)
}

func (t *Term) Print(v ...any) (int, error) {
	t.buffer.write(fmt.Sprint(v...))
	return fmt.Fprint(t.out, v...)
}

func (t *Term) Println(v ...any) (int, error) {
	text := ensureNewline(fmt.Sprintln(v...))
	t.buffer.write(text)
	return fmt.Fprint(t.out, text)
}

func (t *Term) Printf(format string, v ...any) (int, error) {
	text := ensureNewline(fmt.Sprintf(format, v...))
	t.buffer.write(text)
	return fmt.Fprint(t.out, text)
}

func (t *Term) Debug(v ...any) (int, error) {
	if !t.debug {
		return 0, nil
	}
	return t.output(DebugColor, ensurePrefix(fmt.Sprintln(v...), " - "))
}

func (t *Term) Debugf(format string, v ...any) (int, error) {
	if !t.debug {
		return 0, nil
	}
	s := ensureNewline(ensurePrefix(fmt.Sprintf(format, v...), " - "))
	return t.output(DebugColor, s)
}

func (t *Term) Info(v ...any) (int, error) {
	s := ensurePrefix(fmt.Sprintln(v...), " * ")
	return t.output(InfoColor, s)
}

func (t *Term) Infof(format string, v ...any) (int, error) {
	s := ensureNewline(ensurePrefix(fmt.Sprintf(format, v...), " * "))
	return t.output(InfoColor, s)
}

func (t *Term) Warn(v ...any) (int, error) {
	msg := ensurePrefix(fmt.Sprintln(v...), " ! ")
	t.warnings = append(t.warnings, msg)
	return t.output(WarnColor, msg)
}

func (t *Term) Warnf(format string, v ...any) (int, error) {
	msg := ensureNewline(ensurePrefix(fmt.Sprintf(format, v...), " ! "))
	t.warnings = append(t.warnings, msg)
	return t.output(WarnColor, msg)
}

func (t *Term) Error(v ...any) (int, error) {
	msg := fmt.Sprintln(v...)
	t.buffer.write(msg)
	return output(t.err, ErrorColor, msg)
}

func (t *Term) Errorf(format string, v ...any) (int, error) {
	line := ensureNewline(fmt.Sprintf(format, v...))
	t.buffer.write(line)
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
