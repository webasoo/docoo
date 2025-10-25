package redoc

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
		panic("redoc: failed to load embedded assets: " + err.Error())
	}
	return sub
}

// Handler returns an http.Handler that serves a self-contained Redoc viewer.
func Handler(spec []byte) http.Handler {
	specCopy := append([]byte(nil), spec...)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch target := resolveTarget(r.URL.Path); target {
		case "", indexFile:
			if !serveAsset(w, indexFile) {
				http.Error(w, "redoc: index not available", http.StatusInternalServerError)
			}
		case specFile:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(specCopy)
		case "redoc.standalone.js":
			if !serveAsset(w, "redoc.standalone.js") {
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	})
}

// HandlerFromFile loads the OpenAPI document from disk and returns a Redoc handler.
func HandlerFromFile(path string) (http.Handler, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("redoc: read spec %q: %w", path, err)
	}
	return Handler(data), nil
}

// Register adds default handlers under /redoc/ using the provided spec.
func Register(spec []byte) {
	handler := Handler(spec)
	http.Handle("/redoc", handler)
	http.Handle("/redoc/", handler)
}

// RegisterFile is a convenience helper that loads openapi.json from disk and wires the standard routes.
func RegisterFile(path string) error {
	handler, err := HandlerFromFile(path)
	if err != nil {
		return err
	}
	http.Handle("/redoc", handler)
	http.Handle("/redoc/", handler)
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

	if strings.HasPrefix(cleaned, "redoc/") {
		cleaned = strings.TrimPrefix(cleaned, "redoc/")
	}
	if cleaned == "redoc" {
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
	}

	_, _ = w.Write(data)
	return true
}
