package ginswagger

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/webasoo/docoo/swagger"
)

// Handler adapts the Swagger UI handler to Gin.
func Handler(spec []byte) gin.HandlerFunc {
	return gin.WrapH(swagger.Handler(spec))
}

// Register attaches GET handlers for /swagger and /swagger/*any.
func Register(router gin.IRoutes, spec []byte) {
	handler := Handler(spec)
	router.GET("/swagger", handler)
	router.GET("/swagger/*any", handler)
}

// RegisterFile loads an OpenAPI document from disk and mounts the Swagger UI routes for Gin routers.
func RegisterFile(router gin.IRoutes, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("ginswagger: read spec %q: %w", path, err)
	}
	Register(router, data)
	return nil
}
