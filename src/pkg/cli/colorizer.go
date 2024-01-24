package cli

import (
	"fmt"
	"io"
	"os"
	"regexp"

	"golang.org/x/term"
)

var (
	termEnv    = os.Getenv("TERM")
	isTerminal = term.IsTerminal(int(os.Stdout.Fd())) && termEnv != ""
	_, noColor = os.LookupEnv("NO_COLOR") // per spec, the value doesn't matter
	CanColor   = isTerminal && !noColor && termEnv != "dumb"
	DoColor    = CanColor
)

type Color string

const (
	Nop           Color = ""
	HideCursor    Color = "\033[?25l"
	ShowCursor    Color = "\033[?25h"
	Reset         Color = "\033[0m"
	Bright        Color = "\033[1m"
	Dim           Color = "\033[2m"
	Underscore    Color = "\033[4m"
	Blink         Color = "\033[5m"
	Reverse       Color = "\033[7m"
	Hidden        Color = "\033[8m"
	Black         Color = "\033[30m"
	Red           Color = "\033[31m"
	Green         Color = "\033[32m"
	Yellow        Color = "\033[33m"
	Blue          Color = "\033[34m"
	Purple        Color = "\033[35m"
	Cyan          Color = "\033[36m"
	White         Color = "\033[37m"
	Gray          Color = "\033[1;30m"
	BrightRed     Color = "\033[1;31m"
	BrightGreen   Color = "\033[1;32m"
	BrightYellow  Color = "\033[1;33m"
	BrightBlue    Color = "\033[1;34m"
	BrightPurple  Color = "\033[1;35m"
	BrightCyan    Color = "\033[1;36m"
	BrightWhite   Color = "\033[1;37m"
	Transparent   Color = "\033[39m"
	BgBlack       Color = "\033[40m"
	BgYellow      Color = "\033[43m"
	BgBlue        Color = "\033[44m"
	BgMagenta     Color = "\033[45m"
	BgCyan        Color = "\033[46m"
	BgGreen       Color = "\033[42m"
	BgWhite       Color = "\033[47m"
	BgTransparent Color = "\033[49m"

	InfoColor  = BrightPurple
	ErrorColor = BrightRed
	WarnColor  = BrightYellow
	DebugColor = Gray
)

func (c Color) String() string {
	if DoColor {
		return string(c)
	}
	return ""
}

func Fprint(w io.Writer, c Color, v ...any) (int, error) {
	if DoColor && c != Nop {
		fmt.Fprint(w, string(c))
		defer fmt.Fprint(w, Reset) // or append to v?
	}
	return fmt.Fprint(w, v...)
}

func Fprintln(w io.Writer, c Color, v ...any) (int, error) {
	if DoColor && c != Nop {
		fmt.Fprint(w, string(c))
		defer fmt.Fprint(w, Reset) // or append to v?
	}
	return fmt.Fprintln(w, v...)
}

func Print(c Color, v ...any) (int, error) {
	return Fprint(os.Stdout, c, v...)
}

func Println(c Color, v ...any) (int, error) {
	return Fprintln(os.Stdout, c, v...)
}

func Debug(v ...any) (int, error) {
	if !DoVerbose {
		return 0, nil
	}
	return Fprintln(os.Stderr, DebugColor, v...)
}

func Info(v ...any) (int, error) {
	return Println(InfoColor, v...)
}

func Warn(v ...any) (int, error) {
	return Fprintln(os.Stderr, WarnColor, v...)
}

func Error(v ...any) (int, error) {
	return Fprintln(os.Stderr, ErrorColor, v...)
}

// var backgroundColors = []Color{
// 	// BgTransparent,
// 	BgBlack,
// 	BgYellow,
// 	BgBlue,
// 	BgMagenta,
// 	BgCyan,
// 	BgGreen,
// 	BgWhite,
// }

// type colorizer struct {
// 	next  int
// 	color map[string]string
// }

// func (c *colorizer) backgroundColor(id string) string {
// 	if c.color == nil {
// 		c.color = make(map[string]string)
// 	}
// 	if c.color[id] == "" {
// 		c.color[id] = backgroundColors[c.next]
// 		c.next = (c.next + 1) % len(backgroundColors)
// 	}
// 	return c.color[id]
// }

var ansiRegex = regexp.MustCompile("\x1b(?:\\[[=?]?[0-9;]*[@-~]|].*?(?:\x1b\\\\|\x07|$)|[@-Z\\\\^_])")

func StripAnsi(s string) string {
	return ansiRegex.ReplaceAllLiteralString(s, "")
}
