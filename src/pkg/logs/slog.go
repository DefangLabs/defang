package logs

import (
	"context"
	"log/slog"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/term"
)

type termHandler struct {
	t *term.Term
}

func newTermHandler(t *term.Term) *termHandler {
	return &termHandler{t: t}
}

func NewTermLogger(t *term.Term) *slog.Logger {
	return slog.New(newTermHandler(t))
}

func (h *termHandler) Handle(ctx context.Context, r slog.Record) error {
	msg := r.Message
	// Format attrs if any
	if r.NumAttrs() > 0 {
		var builder strings.Builder
		builder.WriteString(msg)
		opened := false
		r.Attrs(func(a slog.Attr) bool {
			if !opened {
				builder.WriteString(" {")
				opened = true
			} else {
				builder.WriteString(", ")
			}
			strVal := a.String()
			if len(strVal) > 80 {
				runes := []rune(strVal)
				strVal = string(runes[:77]) + "..."
			}
			builder.WriteString(strVal)
			return true
		})
		builder.WriteString("}")
		msg = builder.String()
	}

	switch r.Level {
	case slog.LevelDebug:
		_, err := h.t.Debug(msg)
		return err
	case slog.LevelInfo:
		_, err := h.t.Info(msg)
		return err
	case slog.LevelWarn:
		_, err := h.t.Warn(msg)
		return err
	case slog.LevelError:
		_, err := h.t.Error(msg)
		return err
	default:
		_, err := h.t.Println(msg)
		return err
	}
}

func (h *termHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if level == slog.LevelDebug {
		return h.t.DoDebug()
	}
	return true
}

func (h *termHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Since we format attributes in Handle(), we can just return self
	return h
}

func (h *termHandler) WithGroup(name string) slog.Handler {
	// Groups are not supported in this implementation
	return h
}
