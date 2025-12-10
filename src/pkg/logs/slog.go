package logs

import (
	"context"
	"log/slog"

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
	// Format attrs if any
	var attrs string
	if r.NumAttrs() > 0 {
		r.Attrs(func(a slog.Attr) bool {
			if attrs == "" {
				attrs = " {"
			} else {
				attrs += ", "
			}
			strVal := a.String()
			if len(strVal) > 80 {
				strVal = strVal[:77] + "..."
			}
			attrs += strVal
			return true
		})
		attrs += "}"
	}

	msg := r.Message + attrs

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
