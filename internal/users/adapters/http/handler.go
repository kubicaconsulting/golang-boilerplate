// Package userhttp is the code-first HTTP adapter for users.
package userhttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"boilerplate/internal/platform/auth"
	"boilerplate/internal/users/application"
	userdomain "boilerplate/internal/users/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const usersTag = "Users"

// Service is the narrow use-case boundary required by the users HTTP adapter.
type Service interface {
	Create(context.Context, application.Actor, string, string) (userdomain.User, error)
	Get(context.Context, application.Actor, string) (userdomain.User, error)
	List(context.Context, application.Actor, int, int) ([]userdomain.User, error)
}

// Handler translates users requests and application errors into public HTTP contracts.
type Handler struct {
	service Service
	logger  *slog.Logger
}

// NewHandler injects the users use cases and structured error logger.
func NewHandler(service Service, logger *slog.Logger) *Handler { return &Handler{service, logger} }

// User is the public user representation, separate from the domain model.
type User struct {
	ID        string    `json:"id" example:"reference-user-id" doc:"Opaque, server-generated identifier"`
	Name      string    `json:"name" example:"Ada Lovelace"`
	Email     string    `json:"email" format:"email" example:"ada@example.com"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateInput defines code-first create payload validation.
type CreateInput struct {
	Body struct {
		Name  string `json:"name" minLength:"1" maxLength:"100" example:"Ada Lovelace"`
		Email string `json:"email" format:"email" maxLength:"254" example:"ada@example.com"`
	}
}

// GetInput validates the opaque user identifier from the route.
type GetInput struct {
	ID string `path:"id" minLength:"1" maxLength:"128"`
}

// ListInput defines bounded pagination parameters.
type ListInput struct {
	Limit  int `query:"limit" default:"20" minimum:"1" maximum:"100"`
	Offset int `query:"offset" default:"0" minimum:"0" maximum:"100000"`
}

// UserOutput wraps the public response for a single user.
type UserOutput struct{ Body User }

// ListOutput wraps a user page and its effective pagination.
type ListOutput struct {
	Body struct {
		Data   []User `json:"data"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
}

func userResponse(user userdomain.User) User {
	return User{ID: user.ID, Name: user.Name, Email: user.Email, CreatedAt: user.CreatedAt}
}

func actor(ctx context.Context) application.Actor {
	principal, _ := auth.FromContext(ctx)
	return application.Actor{Subject: principal.Subject, Scopes: principal.Scopes}
}

func (h *Handler) failure(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, userdomain.ErrInvalid):
		return huma.Error422UnprocessableEntity("User data is invalid")
	case errors.Is(err, userdomain.ErrConflict):
		return huma.Error409Conflict("A user with that email already exists")
	case errors.Is(err, userdomain.ErrNotFound):
		return huma.Error404NotFound("User not found")
	case errors.Is(err, application.ErrForbidden):
		return huma.Error403Forbidden("Insufficient permissions")
	case errors.Is(err, application.ErrDisabled):
		return huma.Error503ServiceUnavailable("User listing is disabled")
	case errors.Is(err, context.DeadlineExceeded):
		return huma.Error504GatewayTimeout("Operation timed out")
	case errors.Is(err, context.Canceled):
		return huma.NewError(http.StatusRequestTimeout, "Request canceled")
	default:
		h.logger.ErrorContext(ctx, "user operation failed")
		return huma.Error500InternalServerError("Internal server error")
	}
}

// Register keeps operation metadata beside typed handlers; Huma derives OpenAPI schemas.
func (h *Handler) Register(api huma.API, authenticate *auth.Authenticator, maxBody int64) {
	security := []map[string][]string{{"apiKey": {}}, {"bearerAuth": {}}}
	protected := protectedMiddleware(api, authenticate)
	huma.Register(api, huma.Operation{
		OperationID:   "create-user",
		Method:        "POST",
		Path:          "/api/v1/users",
		Summary:       "Create user",
		Description:   "Requires users:write. Email addresses are normalized and unique.",
		Tags:          []string{usersTag},
		DefaultStatus: http.StatusCreated,
		MaxBodyBytes:  maxBody,
		Errors:        []int{401, 403, 409, 413, 422, 500, 504},
		Security:      security,
		Middlewares:   huma.Middlewares{protected},
	}, func(ctx context.Context, in *CreateInput) (*UserOutput, error) {
		user, err := h.service.Create(ctx, actor(ctx), in.Body.Name, in.Body.Email)
		if err != nil {
			return nil, h.failure(ctx, err)
		}
		return &UserOutput{Body: userResponse(user)}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "get-user",
		Method:      "GET",
		Path:        "/api/v1/users/{id}",
		Summary:     "Get user",
		Description: "Requires users:read.",
		Tags:        []string{usersTag},
		Errors:      []int{401, 403, 404, 422, 500, 504},
		Security:    security,
		Middlewares: huma.Middlewares{protected},
	}, func(ctx context.Context, in *GetInput) (*UserOutput, error) {
		user, err := h.service.Get(ctx, actor(ctx), in.ID)
		if err != nil {
			return nil, h.failure(ctx, err)
		}
		return &UserOutput{Body: userResponse(user)}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "list-users",
		Method:      "GET",
		Path:        "/api/v1/users",
		Summary:     "List users",
		Description: "Requires users:read. Ordered by creation time then ID; users-list flag controls " +
			"availability.",
		Tags:        []string{usersTag},
		Errors:      []int{401, 403, 422, 500, 503, 504},
		Security:    security,
		Middlewares: huma.Middlewares{protected},
	}, func(ctx context.Context, in *ListInput) (*ListOutput, error) {
		users, err := h.service.List(ctx, actor(ctx), in.Limit, in.Offset)
		if err != nil {
			return nil, h.failure(ctx, err)
		}
		out := &ListOutput{}
		out.Body.Data = make([]User, 0, len(users))
		for _, user := range users {
			out.Body.Data = append(out.Body.Data, userResponse(user))
		}
		out.Body.Limit = in.Limit
		out.Body.Offset = in.Offset
		return out, nil
	})
}

func protectedMiddleware(api huma.API, authenticate *auth.Authenticator) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		route := attribute.String("http.route", ctx.Operation().Path)
		span := trace.SpanFromContext(ctx.Context())
		span.SetName(ctx.Method() + " " + ctx.Operation().Path)
		span.SetAttributes(route)
		if labeler, ok := otelhttp.LabelerFromContext(ctx.Context()); ok {
			labeler.Add(route)
		}
		request := humagin.Unwrap(ctx).Request
		principal, err := authenticate.Authenticate(request)
		if err != nil {
			ctx.SetHeader("WWW-Authenticate", "Bearer")
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Missing or invalid credentials")
			return
		}
		next(huma.WithContext(ctx, auth.WithPrincipal(ctx.Context(), principal)))
	}
}
