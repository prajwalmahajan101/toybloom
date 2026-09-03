package api

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	ginmiddleware "github.com/oapi-codegen/gin-middleware"
	"github.com/prajwalmahajan101/toybloom/internal/api/gen"
	"github.com/prajwalmahajan101/toybloom/internal/core/config"
	"github.com/prajwalmahajan101/toybloom/internal/core/response"
	"github.com/prajwalmahajan101/toybloom/internal/obs"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func NewRouter(h *Handler, hh *HealthHandler, cfg config.Config, lg *slog.Logger, inst *obs.Instruments) *gin.Engine {
	r := gin.New()
	// Order matters: otelgin FIRST so the server span is active for everything
	// downstream (correlation id derives from its trace id; the metrics
	// exemplar and every log line attach to it). Then correlation id, RED
	// metrics, the request logger, recovery (panics logged with the scoped
	// logger), and finally timeout + body cap.
	r.Use(
		otelgin.Middleware(cfg.ServiceName),
		CorrelationID(),
		HTTPMetrics(inst),
		RequestLogger(lg),
		recoveryHandler(),
		Timeout(cfg.RequestTimeout),
		BodyLimit(cfg.MaxBodyBytes),
	)

	// No /metrics scrape endpoint: metrics leave via OTLP (ADR 0005).
	r.GET("/healthz", hh.Healthz)
	r.GET("/readyz", hh.Readyz)
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
