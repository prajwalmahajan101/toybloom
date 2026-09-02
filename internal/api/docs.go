package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apispec "github.com/prajwalmahajan101/toybloom/api"
	"github.com/swaggest/swgui/v5emb"
)

// registerDocs mounts the OpenAPI contract and a Swagger UI over it:
//   - GET /openapi.yaml  → the raw spec (source of truth for client SDKs)
//   - GET /docs          → Swagger UI (assets embedded by swgui; works offline)
func registerDocs(r *gin.Engine) {
	r.GET("/openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml", apispec.Spec)
	})

	swagger := v5emb.New("toybloom API", "/openapi.yaml", "/docs/")
	r.GET("/docs", func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, "/docs/") })
	r.GET("/docs/*any", gin.WrapH(swagger))
}
