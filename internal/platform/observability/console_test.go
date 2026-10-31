package observability

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConsoleColorPolicy(t *testing.T) {
	for _, test := range []struct {
		name     string
		mode     string
		terminal bool
		noColor  string
		term     string
		want     bool
	}{
		{name: "terminal", mode: "auto", terminal: true, want: true},
		{name: "redirected", mode: "auto"},
		{name: "disabled", mode: "never", terminal: true},
		{name: "NO_COLOR", mode: "auto", terminal: true, noColor: "1"},
		{name: "dumb terminal", mode: "auto", terminal: true, term: "dumb"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", test.noColor)
			t.Setenv("TERM", test.term)
			if got := useConsoleColor(test.mode, test.terminal); got != test.want {
				t.Fatalf("color enabled: got %v, want %v", got, test.want)
			}
		})
	}
}

func TestConsoleGroupsAndDerivedLoggers(t *testing.T) {
	var output bytes.Buffer
	level := new(slog.LevelVar)
	logger := slog.New(newConsoleHandler(&output, level, false)).With("component", "HTTP", "service", "test")
	grouped := logger.WithGroup("request").With("id", 42)
	grouped.Info("accepted", "component", "nested", "method", "GET")
	logger.With("component", "Users").Info("created")
	logger.Info("listening")
	slog.New(newConsoleHandler(&output, level, false)).Info("dynamic", "component", "Database")

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	groupedLine := "[HTTP] accepted service=test request.id=42 request.component=nested request.method=GET"
	if len(lines) != 4 ||
		!strings.Contains(lines[0], groupedLine) ||
		!strings.Contains(lines[1], "[Users] created") ||
		!strings.Contains(lines[2], "[HTTP] listening") ||
		!strings.Contains(lines[3], "[Database] dynamic") {
		t.Fatalf("group or logger state lost: %s", output.String())
	}
	if strings.Contains(output.String(), "\x1b") {
		t.Fatal("plain console output contains terminal codes")
	}

	output.Reset()
	logger.Debug("hidden")
	level.Set(slog.LevelDebug)
	logger.WithGroup("").Debug("visible")
	if strings.Contains(output.String(), "hidden") || !strings.Contains(output.String(), "visible") {
		t.Fatal("live level changes or empty groups are not respected")
	}
}

func TestConsoleLevelsAndEscaping(t *testing.T) {
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		t.Run(level.String(), func(t *testing.T) {
			var output bytes.Buffer
			logger := slog.New(newConsoleHandler(&output, slog.LevelDebug, true))
			logger.Log(t.Context(), level, "line\n\x1b[31m", "component", "HTTP\n", "detail", "value\n\x1b[31m")
			line := output.String()
			if !strings.Contains(line, levelColor(level)) || !strings.Contains(line, "\x1b[0m") {
				t.Fatalf("missing level color: %q", line)
			}
			if strings.Count(line, "\n") != 1 || !strings.Contains(line, `HTTP\n`) || !strings.Contains(line, `line\n\x1b[31m`) {
				t.Fatalf("control characters were not escaped: %q", line)
			}
		})
	}
}

func TestConsoleConcurrentWrites(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(newConsoleHandler(&output, slog.LevelInfo, false))
	var workers sync.WaitGroup
	for i := range 8 {
		workers.Go(func() {
			worker := logger.With("component", "Worker", "worker", i)
			for range 20 {
				worker.Info("completed")
			}
		})
	}
	workers.Wait()
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 160 {
		t.Fatalf("lost or split events: got %d", len(lines))
	}
	for _, line := range lines {
		if !strings.Contains(line, "[Worker] completed worker=") {
			t.Fatalf("interleaved event: %q", line)
		}
	}
}

type failedConsoleWriter struct{}

func (failedConsoleWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestConsoleReportsWriteFailures(t *testing.T) {
	handler := newConsoleHandler(failedConsoleWriter{}, slog.LevelInfo, false)
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	if err := handler.Handle(context.Background(), record); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("output failure lost: %v", err)
	}
}
