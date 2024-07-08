package logs

import (
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
	getMessage := func() string {
		var buf strings.Builder
		buf.WriteString(entry.Message)
		for k, v := range entry.Data {
			buf.WriteString(" ")
			buf.WriteString(k)
			buf.WriteString("=")
			buf.WriteString(v.(string))
		}
		return buf.String()
	}

	switch entry.Level {
	case logrus.PanicLevel, logrus.FatalLevel:
		f.Term.Fatal(getMessage())
	case logrus.ErrorLevel:
		f.Term.Error(getMessage())
	case logrus.WarnLevel:
		f.Term.Warn(getMessage())
	case logrus.InfoLevel:
		f.Term.Info(getMessage())
	case logrus.DebugLevel, logrus.TraceLevel:
		if f.Term.DoDebug() {
			f.Term.Debug(getMessage())
		}
	}

	return nil, nil
}

type DiscardFormatter struct{}

func (f DiscardFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	return nil, nil
}
