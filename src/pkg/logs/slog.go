package logs

import (
	"context"
	"log/slog"

	"github.com/DefangLabs/defang/src/pkg/term"
)

type termHandler struct {
	t           *term.Term
	preformatted string // Pre-formatted attributes with their group prefixes
	groups      []string
}

func newTermHandler(t *term.Term) *termHandler {
	return &termHandler{t: t}
}

func NewTermLogger(t *term.Term) *slog.Logger {
	return slog.New(newTermHandler(t))
}

func (h *termHandler) Handle(ctx context.Context, r slog.Record) error {
	// Start with pre-formatted attributes
	attrs := h.preformatted
	
	// Add attrs from the record (with current group prefix if any)
	r.Attrs(func(a slog.Attr) bool {
		if attrs != "" {
			attrs += ", "
		}
		
		// Add group prefix if any
		prefix := ""
		for _, g := range h.groups {
			prefix += g + "."
		}
		
		attrs += prefix + a.String()
		return true
	})
	
	if attrs != "" {
		attrs = " {" + attrs + "}"
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
	// Format new attributes with current group prefix
	newPreformatted := h.preformatted
	
	prefix := ""
	for _, g := range h.groups {
		prefix += g + "."
	}
	
	for _, a := range attrs {
		if newPreformatted != "" {
			newPreformatted += ", "
		}
		newPreformatted += prefix + a.String()
	}
	
	return &termHandler{
		t:           h.t,
		preformatted: newPreformatted,
		groups:      h.groups,
	}
}

func (h *termHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	
	// Create a new handler with the group added
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name
	
	return &termHandler{
		t:           h.t,
		preformatted: h.preformatted,
		groups:      newGroups,
	}
}
