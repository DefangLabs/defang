package logs

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/DefangLabs/defang/src/pkg/term"
)

type termHandler struct {
	t     *term.Term
	attrs string // pre-formatted persistent attrs
	mu    sync.Mutex
}

func newTermHandler(t *term.Term) *termHandler {
	return &termHandler{t: t}
}

func NewTermLogger(t *term.Term) *slog.Logger {
	return slog.New(newTermHandler(t))
}

func (h *termHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	msg := r.Message

	// Collect attrs from WithAttrs and from this record
	var sb strings.Builder
	if h.attrs != "" {
		sb.WriteString(h.attrs)
	}
	r.Attrs(func(a slog.Attr) bool {
		if sb.Len() > 0 {
			sb.WriteString(", ")
		}
		strVal := a.String()
		if len(strVal) > 80 {
			runes := []rune(strVal)
			strVal = string(runes[:77]) + "..."
		}
		sb.WriteString(strVal)
		return true
	})
	if sb.Len() > 0 {
		msg = msg + " {" + sb.String() + "}"
	}

	switch r.Level {
	case slog.LevelDebug:
		_, err := h.t.WriteDebug(msg)
		return err
	case slog.LevelInfo:
		_, err := h.t.WriteInfo(msg)
		return err
	case slog.LevelWarn:
		_, err := h.t.WriteWarn(msg)
		return err
	case slog.LevelError:
		_, err := h.t.WriteError(msg)
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
	var sb strings.Builder
	sb.WriteString(h.attrs)
	for _, a := range attrs {
		if sb.Len() > 0 {
			sb.WriteString(", ")
		}
		strVal := a.String()
		if len(strVal) > 80 {
			runes := []rune(strVal)
			strVal = string(runes[:77]) + "..."
		}
		sb.WriteString(strVal)
	}
	return &termHandler{t: h.t, attrs: sb.String()}
}

func (h *termHandler) WithGroup(name string) slog.Handler {
	return h
}
