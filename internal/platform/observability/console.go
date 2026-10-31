package observability

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync"
)

const componentKey = "component"

// consoleOutput serializes complete lines across derived loggers.
type consoleOutput struct {
	sync.Mutex
	buffer bytes.Buffer
	writer io.Writer
}

type consoleHandler struct {
	handler   slog.Handler
	output    *consoleOutput
	component string
	grouped   bool
	color     bool
}

func newConsoleHandler(writer io.Writer, level slog.Leveler, color bool) *consoleHandler {
	output := &consoleOutput{writer: writer}
	handler := slog.NewTextHandler(&output.buffer, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if len(groups) == 0 {
				switch attr.Key {
				case slog.TimeKey, slog.LevelKey, slog.MessageKey, componentKey:
					return slog.Attr{}
				}
			}
			return attr
		},
	})
	return &consoleHandler{handler: handler, output: output, color: color}
}

func (h *consoleHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *consoleHandler) Handle(ctx context.Context, record slog.Record) error {
	h.output.Lock()
	defer h.output.Unlock()

	h.output.buffer.Reset()
	if err := h.handler.Handle(ctx, record); err != nil {
		return err
	}
	component := h.component
	if !h.grouped {
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == componentKey {
				component = attr.Value.Resolve().String()
			}
			return true
		})
	}
	if component != "" {
		component = " [" + consoleText(component) + "]"
	}
	attrs := bytes.TrimSuffix(h.output.buffer.Bytes(), []byte("\n"))
	suffix := ""
	if len(attrs) > 0 {
		suffix = " " + string(attrs)
	}
	level := fmt.Sprintf("%-5s", record.Level)
	if h.color {
		level = levelColor(record.Level) + level + "\x1b[0m"
	}
	_, err := fmt.Fprintf(h.output.writer, "%s %s%s %s%s\n",
		record.Time.Format("15:04:05.000"), level, component, consoleText(record.Message), suffix)
	return err
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.handler = h.handler.WithAttrs(attrs)
	if !h.grouped {
		for _, attr := range attrs {
			if attr.Key == componentKey {
				next.component = attr.Value.Resolve().String()
			}
		}
	}
	return &next
}

func (h *consoleHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.handler = h.handler.WithGroup(name)
	next.grouped = true
	return &next
}

// Escape user-controlled control characters while keeping each event on one terminal line.
func consoleText(value string) string {
	quoted := strconv.Quote(value)
	return quoted[1 : len(quoted)-1]
}

func levelColor(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "\x1b[31m"
	case level >= slog.LevelWarn:
		return "\x1b[33m"
	case level >= slog.LevelInfo:
		return "\x1b[32m"
	default:
		return "\x1b[36m"
	}
}

func useConsoleColor(mode string, terminal bool) bool {
	return mode != "never" && terminal && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}
