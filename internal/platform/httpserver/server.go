// Package httpserver provides transport-wide middleware and public system routes.
package httpserver

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync/atomic"
	"time"

	"boilerplate/internal/config"
	"boilerplate/internal/platform/auth"
	userhttp "boilerplate/internal/users/adapters/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/felixge/httpsnoop"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const readinessTimeout = 2 * time.Second

type readiness struct {
	ready atomic.Bool
	check func(context.Context) error
}

// Server owns the HTTP handler and dependency-aware readiness state.
type Server struct {
	Handler http.Handler
	API     huma.API
	state   *readiness
}

type healthOutput struct {
	Body struct {
		Status string `json:"status" enum:"ok"`
	}
}

// NewServer only constructs routes. The application runtime owns listeners and resources.
func NewServer(
	c config.HTTP,
	users *userhttp.Handler,
	a *auth.Authenticator,
	logger *slog.Logger,
	check func(context.Context) error,
) *Server {
	// Our net/http middleware owns logging, recovery and tracing; do not attach Gin defaults.
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.RedirectTrailingSlash = false
	router.UseEscapedPath = true
	_ = router.SetTrustedProxies(nil)
	router.NoRoute(func(ctx *gin.Context) { http.NotFound(ctx.Writer, ctx.Request) })
	router.NoMethod(func(ctx *gin.Context) {
		http.Error(ctx.Writer, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	cfg := huma.DefaultConfig("Microservice API", "1.0.0")
	cfg.DocsRenderer = huma.DocsRendererScalar
	if !c.Docs {
		cfg.DocsPath = ""
		cfg.OpenAPIPath = ""
		cfg.SchemasPath = ""
	}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"apiKey":     {Type: "apiKey", In: "header", Name: "X-API-Key"},
		"bearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
	}
	api := humagin.New(router, cfg)
	state := &readiness{check: check}
	users.Register(api, a, c.MaxBodyBytes)
	huma.Register(api, huma.Operation{
		OperationID: "liveness",
		Method:      "GET",
		Path:        "/livez",
		Summary:     "Process liveness",
		Tags:        []string{"System"},
	}, func(context.Context, *struct{}) (*healthOutput, error) {
		out := &healthOutput{}
		out.Body.Status = "ok"
		return out, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "readiness",
		Method:      "GET",
		Path:        "/readyz",
		Summary:     "Required dependency readiness",
		Tags:        []string{"System"},
		Errors:      []int{503},
	}, func(ctx context.Context, _ *struct{}) (*healthOutput, error) {
		if !state.ready.Load() {
			return nil, huma.Error503ServiceUnavailable("Service is not ready")
		}
		bounded, cancel := context.WithTimeout(ctx, readinessTimeout)
		defer cancel()
		if state.check != nil {
			if err := state.check(bounded); err != nil {
				return nil, huma.Error503ServiceUnavailable("Required dependency unavailable")
			}
		}
		out := &healthOutput{}
		out.Body.Status = "ok"
		return out, nil
	})
	// Preserve net/http GET-to-HEAD handling without adding implicit operations to OpenAPI.
	for _, route := range router.Routes() {
		if route.Method == http.MethodGet {
			router.HEAD(route.Path, route.HandlerFunc)
		}
	}

	// Exclude probes from tracing so health-check volume does not dominate request telemetry.
	handler := otelhttp.NewHandler(
		requestMiddleware(logger, c.MaxBodyBytes, router),
		"http.server",
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/livez" && r.URL.Path != "/readyz"
		}),
	)
	return &Server{Handler: handler, API: api, state: state}
}

// SetReady publishes whether the process is accepting application traffic.
func (s *Server) SetReady(ready bool) { s.state.ready.Store(ready) }

func requestMiddleware(logger *slog.Logger, maxBody int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate trusted IDs instead of allowing arbitrary caller input in logs.
		requestID := rand.Text()
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		metrics := httpsnoop.CaptureMetrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.ErrorContext(r.Context(), "request panic recovered",
						"request_id", requestID,
						"stack", string(debug.Stack()),
					)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		}), w, r)
		level := slog.LevelInfo
		if metrics.Code >= http.StatusInternalServerError {
			level = slog.LevelError
		} else if metrics.Code >= http.StatusBadRequest {
			level = slog.LevelWarn
		}
		// No raw URI/query, forwarded IP, authorization header or request body is logged.
		logger.Log(r.Context(), level, "HTTP request",
			"request_id", requestID,
			"method", r.Method,
			"status", metrics.Code,
			"duration_ms", float64(metrics.Duration)/float64(time.Millisecond),
		)
	})
}
