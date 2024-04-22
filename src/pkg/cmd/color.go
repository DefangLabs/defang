package cmd

import "github.com/defang-io/defang/src/pkg/term"

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
		if term.CanColor {
			return ColorAlways
		}
		fallthrough
	case "always", "never", "raw":
		return Color(color)
	default:
		term.Fatal("invalid color option: " + color)
		panic("unreachable")
	}
}
