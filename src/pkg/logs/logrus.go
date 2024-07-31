package logs

import (
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
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

type TermLogFormatter struct {
	Term *term.Term
}

func (f TermLogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var buf strings.Builder
	buf.WriteString(entry.Message)
	for k, v := range entry.Data {
		fmt.Fprintf(&buf, " %s=%v", k, v)
	}

	switch entry.Level {
	case logrus.PanicLevel, logrus.FatalLevel:
		f.Term.Fatal(buf.String())
	case logrus.ErrorLevel:
		f.Term.Error(buf.String())
	case logrus.WarnLevel:
		f.Term.Warn(buf.String())
	case logrus.InfoLevel:
		f.Term.Info(buf.String())
	case logrus.DebugLevel, logrus.TraceLevel:
		f.Term.Debug(buf.String())
	}

	return nil, nil
}

type DiscardFormatter struct{}

func (f DiscardFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	return nil, nil
}
