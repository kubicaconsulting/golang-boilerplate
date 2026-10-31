// Package observability configures structured logs and optional OTLP signals.
package observability

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"boilerplate/internal/config"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/term"
)

const metricExportInterval = 15 * time.Second

// Runtime owns structured logging and optional signal providers.
type Runtime struct {
	Logger  *slog.Logger
	Level   *slog.LevelVar
	traces  *sdktrace.TracerProvider
	metrics *sdkmetric.MeterProvider
	logs    *sdklog.LoggerProvider
}

// NewRuntime constructs the configured sinks and unwinds partially initialized providers on error.
func NewRuntime(ctx context.Context, c config.Config, version string) (_ *Runtime, err error) {
	r := &Runtime{Level: new(slog.LevelVar)}
	var level slog.Level
	_ = level.UnmarshalText([]byte(c.Logging.Level))
	r.Level.Set(level)

	defer func() {
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), c.Telemetry.ExportTimeout)
			defer cancel()
			_ = r.Close(cleanup)
		}
	}()

	options := &slog.HandlerOptions{Level: r.Level}
	var base slog.Handler
	switch c.Logging.Format {
	case "console":
		color := useConsoleColor(c.Logging.Color, term.IsTerminal(int(os.Stdout.Fd())))
		base = newConsoleHandler(os.Stdout, r.Level, color)
	case "text":
		base = slog.NewTextHandler(os.Stdout, options)
	default:
		base = slog.NewJSONHandler(os.Stdout, options)
	}

	handlers := []slog.Handler{base}
	if c.Telemetry.Enabled {
		serviceResource := resource.NewSchemaless(
			attribute.String("service.name", c.Service.Name),
			attribute.String("service.version", version),
			attribute.String("deployment.environment.name", c.Service.Environment),
		)

		traceExporter, exportErr := otlptracehttp.New(ctx, otlptracehttp.WithTimeout(c.Telemetry.ExportTimeout))
		if exportErr != nil {
			return r, exportErr
		}
		r.traces = sdktrace.NewTracerProvider(
			sdktrace.WithResource(serviceResource),
			sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(c.Telemetry.SampleRatio))),
			sdktrace.WithBatcher(traceExporter, sdktrace.WithExportTimeout(c.Telemetry.ExportTimeout)),
		)

		metricExporter, exportErr := otlpmetrichttp.New(ctx, otlpmetrichttp.WithTimeout(c.Telemetry.ExportTimeout))
		if exportErr != nil {
			return r, exportErr
		}
		r.metrics = sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(serviceResource),
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
				metricExporter,
				sdkmetric.WithInterval(metricExportInterval),
				sdkmetric.WithTimeout(c.Telemetry.ExportTimeout),
			)),
		)

		logExporter, exportErr := otlploghttp.New(ctx, otlploghttp.WithTimeout(c.Telemetry.ExportTimeout))
		if exportErr != nil {
			return r, exportErr
		}
		r.logs = sdklog.NewLoggerProvider(
			sdklog.WithResource(serviceResource),
			sdklog.WithProcessor(sdklog.NewBatchProcessor(
				logExporter,
				sdklog.WithExportTimeout(c.Telemetry.ExportTimeout),
			)),
		)

		handlers = append(handlers, otelslog.NewHandler(c.Service.Name, otelslog.WithLoggerProvider(r.logs)))
		otel.SetTracerProvider(r.traces)
		otel.SetMeterProvider(r.metrics)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
		if exportErr = runtime.Start(); exportErr != nil {
			return r, exportErr
		}
	}

	r.Logger = slog.New(&fanout{handlers: handlers, level: r.Level}).With(
		"service", c.Service.Name,
		"environment", c.Service.Environment,
	)
	return r, nil
}

// Close shuts down signal providers while reserving time for each remaining signal.
func (r *Runtime) Close(ctx context.Context) error {
	var callbacks []func(context.Context) error
	if r.metrics != nil {
		callbacks = append(callbacks, r.metrics.Shutdown)
	}
	if r.traces != nil {
		callbacks = append(callbacks, r.traces.Shutdown)
	}
	if r.logs != nil {
		callbacks = append(callbacks, r.logs.Shutdown)
	}

	var errs []error
	for i, shutdown := range callbacks {
		// Reserve an export opportunity for each signal, including final logs.
		var bounded context.Context
		var cancel context.CancelFunc
		if deadline, ok := ctx.Deadline(); ok {
			remainingSignals := time.Duration(len(callbacks) - i)
			budget := max(time.Until(deadline)/remainingSignals, 0)
			bounded, cancel = context.WithTimeout(ctx, budget)
		} else {
			bounded, cancel = context.WithCancel(ctx)
		}

		errs = append(errs, shutdown(bounded))
		cancel()
	}

	return errors.Join(errs...)
}

type fanout struct {
	handlers []slog.Handler
	level    *slog.LevelVar
}

func (h *fanout) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level.Level() }

func (h *fanout) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	var errs []error
	for _, next := range h.handlers {
		if next.Enabled(ctx, r.Level) {
			errs = append(errs, next.Handle(ctx, r.Clone()))
		}
	}

	return errors.Join(errs...)
}

func (h *fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(h.handlers))
	for i, v := range h.handlers {
		next[i] = v.WithAttrs(attrs)
	}
	return &fanout{next, h.level}
}

func (h *fanout) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(h.handlers))
	for i, v := range h.handlers {
		next[i] = v.WithGroup(name)
	}
	return &fanout{next, h.level}
}

// Discard is useful for tests that exercise application behavior without output.
func Discard() *slog.Logger { return slog.New(slog.DiscardHandler) }
