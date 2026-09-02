package api

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	ginmiddleware "github.com/oapi-codegen/gin-middleware"
	"github.com/prajwalmahajan101/toybloom/internal/api/gen"
	"github.com/prajwalmahajan101/toybloom/internal/core/config"
	"github.com/prajwalmahajan101/toybloom/internal/core/response"
)

func NewRouter(h *Handler, hh *HealthHandler, cfg config.Config, lg *slog.Logger) *gin.Engine {
	r := gin.New()
	// Order matters: correlation id first (everything downstream reads it),
	// then the request logger (puts the scoped logger in ctx), then recovery
	// (so panics are logged with that logger), then timeout + body cap.
	r.Use(
		CorrelationID(),
		RequestLogger(lg),
		Metrics(),
		recoveryHandler(),
		Timeout(cfg.RequestTimeout),
		BodyLimit(cfg.MaxBodyBytes),
	)

	r.GET("/healthz", hh.Healthz)
	r.GET("/readyz", hh.Readyz)
	r.GET("/metrics", prometheusHandler())
	registerDocs(r)

	// Spec-driven request validation (name pattern, n>=1, 0<p<1, required
	// fields) scoped to the generated filter routes. The spec is the single
	// source of truth — validation lives in api/openapi.yaml, not in code.
	registerFilterRoutes(r, h)

	return r
}

// registerFilterRoutes wires the oapi-codegen-generated routes for the filters
// API, fronted by the OpenAPI request validator. Both the validator and the
// generated router render failures through the standard error envelope.
func registerFilterRoutes(r *gin.Engine, h *Handler) {
	swagger, err := gen.GetSpec()
	if err != nil {
		panic("load embedded openapi spec: " + err.Error())
	}
	swagger.Servers = nil // don't reject requests based on the declared server host

	validator := ginmiddleware.OapiRequestValidatorWithOptions(swagger, &ginmiddleware.Options{
		ErrorHandler: func(c *gin.Context, message string, statusCode int) {
			c.AbortWithStatusJSON(statusCode,
				response.NewError(getCorrelationID(c), message,
					response.ErrorDetail{Code: "INVALID_ARGUMENT", Message: message}))
		},
	})

	gen.RegisterHandlersWithOptions(r, h, gen.GinServerOptions{
		Middlewares: []gen.MiddlewareFunc{gen.MiddlewareFunc(validator)},
		ErrorHandler: func(c *gin.Context, err error, statusCode int) {
			c.AbortWithStatusJSON(statusCode,
				response.NewError(getCorrelationID(c), err.Error(),
					response.ErrorDetail{Code: "INVALID_ARGUMENT", Message: err.Error()}))
		},
	})
}
