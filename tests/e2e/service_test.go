//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestServiceLifecycle(t *testing.T) {
	root := filepath.Join("..", "..")
	binary := filepath.Join(t.TempDir(), "service")
	ldflags := strings.Join([]string{
		"-X boilerplate/internal/buildinfo.version=test-release",
		"-X boilerplate/internal/buildinfo.commit=abc123",
		"-X boilerplate/internal/buildinfo.builtAt=2026-10-31T00:00:00Z",
		"-X boilerplate/internal/buildinfo.dirty=false",
	}, " ")
	build := exec.CommandContext(t.Context(), "go", "build", "-ldflags", ldflags, "-o", binary, "./cmd/service")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", output, err)
	}
	versionOutput, versionErr := exec.CommandContext(t.Context(),
		binary,
		"--version",
		"--env",
		"production",
		"--config-dir",
		"/nonexistent-config-directory",
	).CombinedOutput()
	var metadata struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		BuiltAt string `json:"built_at"`
		Dirty   bool   `json:"dirty"`
	}
	if versionErr != nil {
		t.Fatalf("version command: %s %v", versionOutput, versionErr)
	}
	if decodeErr := json.Unmarshal(versionOutput, &metadata); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if metadata.Version != "test-release" ||
		metadata.Commit != "abc123" ||
		metadata.Dirty ||
		metadata.BuiltAt != "2026-10-31T00:00:00Z" {
		t.Fatalf("version metadata: %s %v", versionOutput, versionErr)
	}
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	environment := "test"
	configDir := "configs"
	if os.Getenv("E2E_DATABASE_DSN") != "" {
		environment = "production"
		configDir = filepath.Join(t.TempDir(), "missing")
	}
	cmd := exec.CommandContext(ctx, binary, "--env", environment, "--config-dir", configDir)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "APP_HTTP_ADDRESS="+address, "APP_LOGGING_LEVEL=info", "APP_LOGGING_FORMAT=json")
	if dsn := os.Getenv("E2E_DATABASE_DSN"); dsn != "" {
		// Use a separate credential in deployments; the public local key must be rejected.
		digest := sha256.Sum256([]byte("e2e-private-credential"))
		cmd.Env = append(
			cmd.Env,
			"APP_DATABASE_DRIVER=postgres",
			"APP_DATABASE_DSN="+dsn,
			`APP_AUTH_API_KEYS=[{"id":"e2e","sha256":"`+hex.EncodeToString(digest[:])+
				`","subject":"e2e","scopes":["users:read","users:write"]}]`,
		)
		migrate := exec.CommandContext(ctx, binary, "--env", environment, "--config-dir", configDir, "--migrate")
		migrate.Dir = root
		migrate.Env = cmd.Env
		if output, migrationErr := migrate.CombinedOutput(); migrationErr != nil {
			t.Fatalf("migrate: %s %v", output, migrationErr)
		}
	}
	var logs lockedBuffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	base := "http://" + address
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	ready := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, base+"/readyz", nil)
		if e != nil {
			t.Fatal(e)
		}
		res, e := client.Do(req)
		if e == nil {
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("not ready: %s", logs.String())
	}
	assertStartupLogs(t, logs.String())
	call := func(method, path, body string) (int, []byte) {
		t.Helper()
		req, e := http.NewRequestWithContext(ctx, method, base+path, strings.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		key := "local-development-key"
		if environment == "production" {
			key = "e2e-private-credential"
		}
		req.Header.Set("X-Api-Key", key)
		req.Header.Set("Content-Type", "application/json")
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = res.Body.Close() }()
		data, e := io.ReadAll(res.Body)
		if e != nil {
			t.Fatal(e)
		}
		return res.StatusCode, data
	}
	code, data := call(
		"POST",
		"/api/v1/users",
		`{"name":"Ada","email":"e2e-`+time.Now().Format("150405.000000000")+`@example.com"}`,
	)
	if code != 201 {
		t.Fatalf("create %d %s", code, data)
	}
	var u struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(data, &u); err != nil {
		t.Fatal(err)
	}
	if code, data = call("GET", "/api/v1/users/"+u.ID, ""); code != 200 {
		t.Fatalf("get %d %s", code, data)
	}
	if err = cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if code, _ = call("GET", "/livez", ""); code != 200 {
		t.Fatal("reload interrupted serving")
	}
	client.CloseIdleConnections()
	if err = cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-exited:
		if err != nil {
			t.Fatalf("shutdown: %v %s", err, logs.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("SIGTERM shutdown timed out: %s", logs.String())
	}
}

func assertStartupLogs(t *testing.T, output string) {
	t.Helper()
	expected := map[string]string{
		"observability initialized":  "Observability",
		"persistence initialized":    "Database",
		"authentication initialized": "Authentication",
		"feature flags initialized":  "FeatureFlags",
		"users feature initialized":  "Users",
		"HTTP routes registered":     "HTTP",
		"application initialized":    "Application",
	}
	decoder := json.NewDecoder(strings.NewReader(output))
	initialized := false
	ready := false
	for {
		var event struct {
			Message   string `json:"msg"`
			Component string `json:"component"`
			Duration  *int64 `json:"duration"`
		}
		err := decoder.Decode(&event)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("startup logs are not JSON: %v", err)
		}
		if component, ok := expected[event.Message]; ok {
			if event.Component != component || event.Duration == nil || *event.Duration < 0 {
				t.Fatalf("initialization event lost component or timing: %+v", event)
			}
			delete(expected, event.Message)
		}
		if event.Message == "application initialized" {
			initialized = true
		}
		if event.Message == "service ready" {
			ready = initialized && event.Component == "Application"
		}
	}
	if len(expected) != 0 || !ready || strings.Contains(output, "\x1b") {
		t.Fatalf("missing startup milestones or invalid readiness order: %v; ready=%v", expected, ready)
	}
}
