package command

import "fmt"

type ColorMode string

const (
	// ColorNever disables color output.
	ColorNever ColorMode = "never"
	// ColorAuto enables color output only if the output is connected to a terminal.
	ColorAuto ColorMode = "auto"
	// ColorAlways enables color output.
	ColorAlways ColorMode = "always"
	// ColorRaw disables color output and does not escape any characters.
	// ColorRaw ColorMode = "raw"
)

var allColorModes = []ColorMode{
	ColorNever,
	ColorAuto,
	ColorAlways,
}

func (c ColorMode) String() string {
	return string(c)
}

func (c *ColorMode) Set(value string) error {
	for _, colorMode := range allColorModes {
		if colorMode.String() == value {
			*c = colorMode
			return nil
		}
	}
	return fmt.Errorf("invalid color: %q, not one of %v", value, allColorModes)
}

func (c ColorMode) Type() string {
	return "color-mode"
}
