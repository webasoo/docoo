package fiberscalar

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"

	"github.com/webasoo/docoo/scalar"
)

// Handler returns a Fiber handler that mounts the Scalar UI under the request path.
func Handler(spec []byte) fiber.Handler {
	return adaptor.HTTPHandler(scalar.Handler(spec))
}

// RegisterWithSpec attaches GET handlers for /scalar and /scalar/* to the provided app using the given document.
func RegisterWithSpec(app *fiber.App, spec []byte) {
	wrapped := Handler(spec)
	app.Get("/scalar", wrapped)
	app.Get("/scalar/*", wrapped)
}

// Register loads openapi.json from the project root and mounts the Scalar UI routes.
func Register(app *fiber.App) error {
	path, err := defaultSpecPath()
	if err != nil {
		return err
	}
	return RegisterFile(app, path)
}

// RegisterFile loads an OpenAPI document from disk and mounts the Scalar UI routes.
func RegisterFile(app *fiber.App, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("fiberscalar: read spec %q: %w", path, err)
	}
	RegisterWithSpec(app, data)
	return nil
}

func defaultSpecPath() (string, error) {
	root, err := findModuleRoot(".")
	if err != nil {
		if root, err = os.Getwd(); err != nil {
			return "", fmt.Errorf("fiberscalar: resolve workspace root: %w", err)
		}
	}
	return filepath.Join(root, "openapi.json"), nil
}

func findModuleRoot(start string) (string, error) {
	abspath, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	dir := abspath
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("fiberscalar: go.mod not found above %s", start)
}
