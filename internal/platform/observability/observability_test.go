package observability_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"boilerplate/internal/config"
	"boilerplate/internal/platform/observability"

	"go.opentelemetry.io/otel"
	logpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestOTLPAllSignals(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	var traceID, logID []byte
	var logComponent string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/v1/traces":
			var req tracepb.ExportTraceServiceRequest
			if decodeErr := proto.Unmarshal(data, &req); decodeErr != nil {
				t.Error(decodeErr)
			}
			for _, res := range req.GetResourceSpans() {
				for _, scope := range res.GetScopeSpans() {
					for _, span := range scope.GetSpans() {
						if span.GetName() == "test-operation" {
							seen["traces"] = true
							traceID = span.GetTraceId()
						}
					}
				}
			}
		case "/v1/logs":
			var req logpb.ExportLogsServiceRequest
			if decodeErr := proto.Unmarshal(data, &req); decodeErr != nil {
				t.Error(decodeErr)
			}
			for _, res := range req.GetResourceLogs() {
				for _, scope := range res.GetScopeLogs() {
					for _, record := range scope.GetLogRecords() {
						if record.GetBody().GetStringValue() == "test log" {
							seen["logs"] = true
							logID = record.GetTraceId()
							for _, attr := range record.GetAttributes() {
								if attr.GetKey() == "component" {
									logComponent = attr.GetValue().GetStringValue()
								}
							}
						}
					}
				}
			}
		case "/v1/metrics":
			var req metricpb.ExportMetricsServiceRequest
			if decodeErr := proto.Unmarshal(data, &req); decodeErr != nil {
				t.Error(decodeErr)
			}
			for _, res := range req.GetResourceMetrics() {
				for _, scope := range res.GetScopeMetrics() {
					for _, metric := range scope.GetMetrics() {
						if metric.GetName() == "test.counter" {
							seen["metrics"] = true
						}
					}
				}
			}
		default:
			t.Errorf("unexpected OTLP path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", receiver.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "")
	c := config.Defaults()
	c.Logging.Format = "console"
	c.Telemetry.Enabled = true
	c.Telemetry.SampleRatio = 1
	r, err := observability.NewRuntime(context.Background(), c, "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, span := otel.Tracer("test").Start(context.Background(), "test-operation")
	r.Logger.With("component", "Workers").InfoContext(ctx, "test log")
	counter, err := otel.Meter("test").Int64Counter("test.counter")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(ctx, 1)
	span.End()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = r.Close(shutdown); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, signal := range []string{"traces", "logs", "metrics"} {
		if !seen[signal] {
			t.Errorf("missing %s", signal)
		}
	}
	if logComponent != "Workers" {
		t.Fatalf("structured component lost in OTLP: %q", logComponent)
	}
	if string(traceID) != string(logID) || len(traceID) == 0 {
		t.Fatal("log and trace are not correlated")
	}
}
