package logs

import (
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/sirupsen/logrus"
)

func IsLogrusError(message string) bool {
	// Logrus's TextFormatter prefixes messages with the uppercase log level, optionally truncated and/or in color
	switch message[:pkg.Min(len(message), 4)] {
	case "ERRO", "FATA", "PANI", "\x1b[31": // red
		return true // always show
	case "WARN", "\x1b[33": // yellow
		fallthrough
	case "TRAC", "DEBU", "\x1b[37": // gray
		fallthrough
	case "", "INFO", "\x1b[36": // blue
		return false // only shown with --verbose
	default:
		return true // show by default (likely Dockerfile errors)
	}
}

type InterceptingLogrusFormatter struct{}

func (f *InterceptingLogrusFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	return nil, nil
}
