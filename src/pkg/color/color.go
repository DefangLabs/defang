package color

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

var AllColorModes = []ColorMode{
	ColorNever,
	ColorAuto,
	ColorAlways,
}

func (c ColorMode) String() string {
	return string(c)
}

func (c *ColorMode) Set(value string) error {
	for _, colorMode := range AllColorModes {
		if colorMode.String() == value {
			*c = colorMode
			return nil
		}
	}
	return fmt.Errorf("color mode not one of %v", AllColorModes)
}

func (c ColorMode) Type() string {
	return "color-mode"
}
