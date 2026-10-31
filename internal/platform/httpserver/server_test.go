package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"boilerplate/internal/config"
	"boilerplate/internal/platform/auth"
	"boilerplate/internal/platform/featureflags"
	"boilerplate/internal/platform/httpserver"
	"boilerplate/internal/platform/observability"
	userhttp "boilerplate/internal/users/adapters/http"
	"boilerplate/internal/users/adapters/memory"
	"boilerplate/internal/users/application"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"go.opentelemetry.io/otel/trace"
)

func server(t *testing.T) *httpserver.Server {
	t.Helper()
	c, err := config.Load("../../../configs", "test")
	if err != nil {
		t.Fatal(err)
	}
	a, err := auth.NewAuthenticator(context.Background(), c.Auth)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close(context.Background()) })
	logger := observability.Discard()
	flags, _ := featureflags.NewProvider(c.Flags, logger)
	service := application.NewService(memory.NewRepository(), flags)
	return httpserver.NewServer(c.HTTP, userhttp.NewHandler(service, logger), a, logger, nil)
}

func request(s *httpserver.Server, method, path, body, key string) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if key != "" {
		r.Header.Set("X-Api-Key", key)
	}
	w := httptest.NewRecorder()
	s.Handler.ServeHTTP(w, r)
	return w
}

func TestUsersHTTP(t *testing.T) {
	s := server(t)
	key := "local-development-key"
	if w := request(s, "GET", "/api/v1/users", "", ""); w.Code != 401 ||
		!strings.Contains(w.Header().Get("Content-Type"), "problem+json") {
		t.Fatalf("authentication: %d %s", w.Code, w.Body.String())
	}
	w := request(s, "POST", "/api/v1/users", `{"name":"Ada","email":"ADA@example.com"}`, key)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var user userhttp.User
	if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	if user.ID == "" || user.Email != "ada@example.com" {
		t.Fatal(user)
	}
	if w = request(s, "GET", "/api/v1/users/"+user.ID, "", key); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = request(s, "POST", "/api/v1/users", `{"name":"Ada","email":"ada@example.com"}`, key); w.Code != 409 {
		t.Fatal(w.Code)
	}
	// Omitted fields must never inherit a previous request's decoded DTO state.
	if w = request(s, "POST", "/api/v1/users", `{"name":"Grace"}`, key); w.Code != 422 {
		t.Fatalf("request state retained: %d %s", w.Code, w.Body.String())
	}
	for _, body := range []string{
		`{`,
		`{"name":"","email":"bad"}`,
		`{"name":"Ada","email":"a@example.com","unexpected":true}`,
	} {
		if w = request(s, "POST", "/api/v1/users", body, key); w.Code < 400 || w.Code >= 500 {
			t.Fatalf("validation: %d %s", w.Code, w.Body.String())
		}
	}
	if w = request(s, "GET", "/api/v1/users?limit=101", "", key); w.Code != 422 {
		t.Fatal(w.Code)
	}
	if w = request(s, "GET", "/api/v1/users/missing", "", key); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w = request(s, "GET", "/api/v1/users", "", key); w.Code != 200 ||
		!bytes.Contains(w.Body.Bytes(), []byte(`"limit":20`)) {
		t.Fatalf("list: %s", w.Body.String())
	}
	if w = request(s, "POST", "/api/v1/users", `{"name":"`+strings.Repeat("a", 2<<20)+`"}`, key); w.Code != 413 {
		t.Fatalf("body limit: %d", w.Code)
	}
	if w = request(s, "GET", "/readyz", "", ""); w.Code != 503 {
		t.Fatal(w.Code)
	}
	s.SetReady(true)
	if w = request(s, "GET", "/readyz", "", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = request(s, "GET", "/livez", "", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
}

func TestConcurrentRequests(t *testing.T) {
	s := server(t)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			body := fmt.Sprintf(`{"name":"User %d","email":"u%d@example.com"}`, i, i)
			w := request(s, http.MethodPost, "/api/v1/users", body, "local-development-key")
			if w.Code != 201 {
				t.Errorf("create %d: %d", i, w.Code)
			}
		})
	}
	wg.Wait()
}

func TestOpenAPIContract(t *testing.T) {
	s := server(t)
	spec := s.API.OpenAPI()
	if spec.Paths["/api/v1/users"].Post.Responses["201"] == nil || len(spec.Paths["/api/v1/users"].Post.Security) != 2 {
		t.Fatal("contract missing response/security")
	}
	if spec.Paths["/readyz"].Get.Responses["503"] == nil {
		t.Fatal("missing readiness failure")
	}
}

func TestGinRoutingCompatibility(t *testing.T) {
	server := server(t)
	for _, scenario := range []struct {
		name, method, path, key string
		status                  int
	}{
		{"head health", http.MethodHead, "/livez", "", http.StatusOK},
		{"head requires authentication", http.MethodHead, "/api/v1/users", "", http.StatusUnauthorized},
		{"head users", http.MethodHead, "/api/v1/users", "local-development-key", http.StatusOK},
		{"unknown route", http.MethodGet, "/missing", "", http.StatusNotFound},
		{"trailing slash", http.MethodGet, "/api/v1/users/", "local-development-key", http.StatusNotFound},
		{"wrong method", http.MethodDelete, "/api/v1/users", "local-development-key", http.StatusMethodNotAllowed},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			response := request(server, scenario.method, scenario.path, "", scenario.key)
			if response.Code != scenario.status {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			if response.Header().Get("X-Request-ID") == "" {
				t.Fatal("request middleware was bypassed")
			}
			if scenario.status == http.StatusMethodNotAllowed {
				methods := strings.Split(response.Header().Get("Allow"), ", ")
				for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
					if !slices.Contains(methods, method) {
						t.Fatalf("Allow header missing %s: %v", method, methods)
					}
				}
			}
		})
	}
}

func TestGinAdapterContextAndRecovery(t *testing.T) {
	server := server(t)
	huma.Register(server.API, huma.Operation{
		OperationID: "context-probe", Method: http.MethodGet, Path: "/test/context/{id}",
		DefaultStatus: http.StatusOK,
		Middlewares: huma.Middlewares{func(ctx huma.Context, next func(huma.Context)) {
			ginContext := humagin.Unwrap(ctx)
			if ginContext.ClientIP() != "127.0.0.1" {
				t.Error("untrusted forwarded client address was accepted")
			}
			if ginContext.Param("id") != "opaque/id" {
				t.Errorf("escaped parameter: %q", ginContext.Param("id"))
			}
			if !trace.SpanFromContext(ginContext.Request.Context()).SpanContext().IsValid() {
				t.Error("trace context did not reach Gin")
			}
			next(huma.WithContext(ctx, auth.WithPrincipal(ctx.Context(), auth.Principal{Subject: "probe"})))
		}},
	}, func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		principal, ok := auth.FromContext(ctx)
		if !ok || principal.Subject != "probe" {
			t.Error("principal did not reach typed handler")
		}
		return &struct{}{}, nil
	})
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{1},
	})
	ctx := trace.ContextWithSpanContext(t.Context(), spanContext)
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/test/context/opaque%2Fid", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.99")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("context probe: %d %s", response.Code, response.Body.String())
	}

	huma.Register(server.API, huma.Operation{OperationID: "panic-probe", Method: http.MethodGet, Path: "/test/panic"},
		func(context.Context, *struct{}) (*struct{}, error) { panic("private panic details") })
	response = request(server, http.MethodGet, "/test/panic", "", "")
	if response.Code != http.StatusInternalServerError ||
		strings.Contains(response.Body.String(), "private panic details") {
		t.Fatalf("unsafe recovery: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("recovery lost request metadata")
	}
	if healthy := request(server, http.MethodGet, "/livez", "", ""); healthy.Code != http.StatusOK {
		t.Fatal("panic interrupted subsequent requests")
	}
}
