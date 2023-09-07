package cmd

import (
	"os"

	"golang.org/x/term"
)

var (
	termEnv    = os.Getenv("TERM")
	isTerminal = term.IsTerminal(int(os.Stdout.Fd())) && termEnv != ""
	_, noColor = os.LookupEnv("NO_COLOR") // per spec, the value doesn't matter
	CanColor   = isTerminal && !noColor && termEnv != "dumb"
)

type Color string

const (
	ColorAuto   Color = "auto"
	ColorAlways Color = "always"
	ColorNever  Color = "never"
	ColorRaw    Color = "raw"
)

func ParseColor(color string) Color {
	switch color {
	case "auto":
		if CanColor {
			return ColorAlways
		}
		fallthrough
	case "always", "never", "raw":
		return Color(color)
	default:
		Fatal("invalid color option: " + color)
		panic("unreachable")
	}
}
