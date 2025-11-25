package crun

import (
	"os"
	"strconv"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type Region = aws.Region

func ParseMemory(memory string) uint64 {
	i := strings.IndexAny(memory, "BKMGIbkmgi")
	var memUnit uint64 = 1
	if i > 0 {
		switch strings.ToUpper(memory[i:]) {
		default:
			term.Fatal("invalid suffix: " + memory)
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
		term.Fatal(err.Error())
	}
	return memoryB * memUnit
}

func ParseEnvLine(line string) (key string, value string) {
	parts := strings.SplitN(line, "=", 2)
	key = strings.TrimSpace(parts[0]) // FIXME: docker only trims leading whitespace
	if key == "" || key[0] == '#' {
		return "", ""
	}
	if len(parts) == 1 {
		var ok bool
		if value, ok = os.LookupEnv(key); !ok { // exclude missing env vars, like docker does
			return "", ""
		}
	} else {
		value = parts[1]
	}
	return
}

func parseEnvFile(content string, env map[string]string) map[string]string {
	if env == nil {
		env = make(map[string]string)
	}
	for _, line := range strings.Split(content, "\n") {
		key, value := ParseEnvLine(strings.TrimSuffix(line, "\r")) // handle CRLF
		if key == "" {
			continue
		}
		env[key] = value
	}
	return env
}

func ParseEnvFile(filename string, env map[string]string) (map[string]string, error) {
	bytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return parseEnvFile(string(bytes), env), nil
}
