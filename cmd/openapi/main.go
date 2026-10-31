// Command openapi generates the contract from the same typed routes used at runtime.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"boilerplate/internal/config"
	"boilerplate/internal/platform/auth"
	"boilerplate/internal/platform/featureflags"
	"boilerplate/internal/platform/httpserver"
	"boilerplate/internal/platform/observability"
	userhttp "boilerplate/internal/users/adapters/http"
	"boilerplate/internal/users/adapters/memory"
	"boilerplate/internal/users/application"

	"github.com/gin-gonic/gin"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	// Emit only the generated JSON, never Gin route-registration diagnostics.
	gin.SetMode(gin.ReleaseMode)

	c := config.Defaults()
	log := observability.Discard()
	flags, err := featureflags.NewProvider(c.Flags, log)
	if err != nil {
		return err
	}
	svc := application.NewService(memory.NewRepository(), flags)
	server := httpserver.NewServer(c.HTTP, userhttp.NewHandler(svc, log), &auth.Authenticator{}, log, nil)
	data, err := json.MarshalIndent(server.API.OpenAPI(), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = os.Stdout.Write(data)
	return err
}
