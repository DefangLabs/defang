package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/defang-io/defang/cli/pkg/aws/ecs"
)

type Region = ecs.Region

func Fatal(msg any) {
	fmt.Println("Error:", msg) // TODO: color red
	os.Exit(1)
}

func ParseMemory(memory string) uint64 {
	i := strings.IndexAny(memory, "BKMGIbkmgi")
	var memUnit uint64 = 1
	if i > 0 {
		switch strings.ToUpper(memory[i:]) {
		default:
			Fatal("invalid suffix: " + memory)
		case "G", "GIB":
			memUnit = 1024 * 1024 * 1024
		case "GB":
			memUnit = 1000 * 1000 * 1000
		case "M", "MIB":
			memUnit = 1024 * 1024
		case "MB":
			memUnit = 1000 * 1000
		case "K", "KIB":
			memUnit = 1024
		case "KB":
			memUnit = 1000
		case "B":
		}
		memory = memory[:i]
	}
	memoryB, err := strconv.ParseUint(memory, 10, 64)
	if err != nil {
		Fatal(err.Error())
	}
	return memoryB * memUnit
}
