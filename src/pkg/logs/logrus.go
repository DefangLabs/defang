package logs

import "github.com/defang-io/defang/src/pkg"

func IsLogrusError(message string) bool {
	// Logrus's TextFormatter prefixes messages with the uppercase log level, optionally truncated and/or in color
	switch message[:pkg.Min(len(message), 4)] {
	case "WARN", "ERRO", "FATA", "PANI", "\x1b[31", "\x1b[33": // red or yellow
		return true // always show
	case "", "INFO", "TRAC", "DEBU", "\x1b[36", "\x1b[37": // blue or gray
		return false // only shown with --verbose
	default:
		return true // show by default (likely Dockerfile errors)
	}
}
