package swagger

import (
	"bytes"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
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
		panic("swagger: failed to load embedded assets: " + err.Error())
	}
	return sub
}

// Handler returns an http.Handler that serves Swagger UI assets and the provided spec.
func Handler(spec []byte) http.Handler {
	return HandlerWithOptions(spec, UIOptions{})
}

// UIOptions controls runtime Swagger UI tweaks applied when serving the index.
type UIOptions struct {
	PersistAuthorization bool
}

// HandlerWithOptions returns an http.Handler that serves Swagger UI assets and the provided spec
// while applying `opts` when serving the index page (for example injecting persistAuthorization).
func HandlerWithOptions(spec []byte, opts UIOptions) http.Handler {
	specCopy := append([]byte(nil), spec...)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch target := resolveTarget(r.URL.Path); target {
		case "", indexFile:
			// load the embedded index and replace the placeholder token with true/false
			data, err := fs.ReadFile(assetFS, indexFile)
			if err != nil {
				http.Error(w, "swagger: index not available", http.StatusInternalServerError)
				return
			}
			replaced := bytes.ReplaceAll(data, []byte("PERSIST_AUTH_PLACEHOLDER"), []byte(strconv.FormatBool(opts.PersistAuthorization)))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(replaced)
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

// HandlerFromFile loads the OpenAPI document from disk and returns a Swagger UI handler.
func HandlerFromFile(path string, opts UIOptions) (http.Handler, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("swagger: read spec %q: %w", path, err)
	}
	return HandlerWithOptions(data, opts), nil
}

// Register adds default handlers under /swagger/ using the provided spec.
func Register(spec []byte) {
	RegisterWithOptions(spec, UIOptions{})
}

// RegisterWithOptions registers the Swagger UI handlers under /swagger using the provided spec
// and applies the given UI options when serving the index page.
func RegisterWithOptions(spec []byte, opts UIOptions) {
	handler := HandlerWithOptions(spec, opts)
	http.Handle("/swagger", handler)
	http.Handle("/swagger/", handler)
}

// RegisterFileWithOptions loads openapi.json from disk and wires the standard routes
// while applying the provided UI options when serving the index page.
func RegisterFileWithOptions(path string, opts UIOptions) error {
	handler, err := HandlerFromFile(path, opts)
	if err != nil {
		return err
	}
	http.Handle("/swagger", handler)
	http.Handle("/swagger/", handler)
	return nil
}

// RegisterFile is a convenience helper that loads openapi.json from disk and wires the standard routes.
func RegisterFile(path string) error {
	return RegisterFileWithOptions(path, UIOptions{})
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

	if strings.HasPrefix(cleaned, "swagger/") {
		cleaned = strings.TrimPrefix(cleaned, "swagger/")
	}
	if cleaned == "swagger" {
		return ""
	}
	return cleaned
}

func serveAsset(w http.ResponseWriter, name string) bool {
	data, err := fs.ReadFile(assetFS, name)
	if err != nil {
		return false
	}

	if ctype := contentTypeFor(name); ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	_, _ = w.Write(data)
	return true
}

func contentTypeFor(name string) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript"
	case ".png":
		return "image/png"
	case ".html":
		return "text/html; charset=utf-8"
	default:
		return mime.TypeByExtension(ext)
	}
}
