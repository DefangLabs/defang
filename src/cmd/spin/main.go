package main

import (
	"fmt"
	"time"

	"github.com/defang-io/defang/src/pkg/spinner"
)

func main() {
	spin := spinner.New()
	for i := 0; i < 20; i++ {
		fmt.Printf(spin.Next())
		time.Sleep(300 * time.Millisecond)
	}

	spinner.SpinnerChars = `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`
	for i := 0; i < 20; i++ {
		fmt.Printf(spin.Next())
		time.Sleep(300 * time.Millisecond)
	}
}
