package main

import "fmt"

type ColorMode string

const (
	// ColorNone disables color output.
	ColorNone ColorMode = "none"
	// ColorAuto enables color output only if the output is connected to a terminal.
	ColorAuto ColorMode = "auto"
	// ColorAlways enables color output.
	ColorAlways ColorMode = "always"
)

var allColorModes = []ColorMode{
	ColorNone,
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
	return fmt.Errorf("color mode not one of %v", allColorModes)
}

func (c ColorMode) Type() string {
	return "color-mode"
}
