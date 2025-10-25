package chiswagger

import (
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/webasoo/docoo/swagger"
)

// Handler exposes the underlying net/http handler for advanced routing setups.
func Handler(spec []byte) http.Handler {
	return swagger.Handler(spec)
}

// Register wires the Swagger UI under /swagger for the provided chi router.
func Register(router chi.Router, spec []byte) {
	handler := Handler(spec)
	router.Handle("/swagger", handler)
	router.Handle("/swagger/*", handler)
}

// RegisterFile loads an OpenAPI document from disk and mounts the Swagger UI routes.
func RegisterFile(router chi.Router, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("chiswagger: read spec %q: %w", path, err)
	}
	Register(router, data)
	return nil
}
