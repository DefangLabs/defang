package cmd

import (
	"fmt"
	"os"
)

const (
	stack = "dev"
)

func Fatal(msg any) {
	fmt.Println("Error:", msg) // TODO: color red
	os.Exit(1)
}
