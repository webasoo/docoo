package scalar

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

const (
	specFile  = "openapi.json"
	indexFile = "index.html"
)

var assetFS = initAssetFS()

func initAssetFS() fs.FS {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		panic("scalar: failed to load embedded assets: " + err.Error())
	}
	return sub
}

// Handler returns an http.Handler that serves the Scalar API reference UI.
func Handler(spec []byte) http.Handler {
	specCopy := append([]byte(nil), spec...)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch target := resolveTarget(r.URL.Path); target {
		case "", indexFile:
			if !serveAsset(w, indexFile) {
				http.Error(w, "scalar: index not available", http.StatusInternalServerError)
			}
		case specFile:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(specCopy)
		default:
			if !serveAsset(w, target) {
				http.NotFound(w, r)
			}
		}
	})
}

// HandlerFromFile loads the OpenAPI document from disk and returns a Scalar handler.
func HandlerFromFile(path string) (http.Handler, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scalar: read spec %q: %w", path, err)
	}
	return Handler(data), nil
}

// Register mounts the Scalar handler under /scalar and /scalar/.
func Register(spec []byte) {
	handler := Handler(spec)
	http.Handle("/scalar", handler)
	http.Handle("/scalar/", handler)
}

// RegisterFile loads openapi.json from disk and mounts the standard Scalar routes.
func RegisterFile(path string) error {
	handler, err := HandlerFromFile(path)
	if err != nil {
		return err
	}
	http.Handle("/scalar", handler)
	http.Handle("/scalar/", handler)
	return nil
}

func resolveTarget(raw string) string {
	cleaned := raw
	if idx := strings.Index(cleaned, "?"); idx >= 0 {
		cleaned = cleaned[:idx]
	}
	if cleaned == "" {
		return ""
	}
	cleaned = path.Clean(cleaned)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." {
		return ""
	}

	if strings.HasPrefix(cleaned, "scalar/") {
		cleaned = strings.TrimPrefix(cleaned, "scalar/")
	}
	if cleaned == "scalar" {
		return ""
	}
	return cleaned
}

func serveAsset(w http.ResponseWriter, name string) bool {
	data, err := fs.ReadFile(assetFS, name)
	if err != nil {
		return false
	}

	switch path.Ext(name) {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}

	_, _ = w.Write(data)
	return true
}
