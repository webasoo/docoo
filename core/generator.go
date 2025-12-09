package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProjectConfig describes how the OpenAPI document should be generated for a project tree.
// All fields are optional; zero values trigger automatic discovery based on the current module.
type ProjectConfig struct {
	WorkspaceRoot string   // module/workspace root; defaults to the current module root
	RoutePaths    []string // directories to scan for routes; defaults to WorkspaceRoot
	SkipPrefixes  []string // path prefixes (e.g. /swagger) to ignore from the generated spec
	OutputPath    string   // destination for GenerateAndSaveOpenAPI; relative paths resolved against WorkspaceRoot
	ProjectName   string   // optional override for the generated document title/tagline
	EnableAuthUI  bool     // include Bearer auth + global security requirement in generated OpenAPI doc
}

// GenerateProjectOpenAPI discovers routes and handlers for the current project and returns
// the generated OpenAPI document. When no configuration is provided it automatically detects
// the module root and scans it for routes.
func GenerateProjectOpenAPI(configs ...ProjectConfig) ([]byte, error) {
	var cfg ProjectConfig
	if len(configs) > 0 {
		cfg = configs[0]
	}

	root, err := resolveWorkspaceRoot(cfg.WorkspaceRoot)
	if err != nil {
		return nil, err
	}

	routeInputs, err := resolveRouteInputs(root, cfg.RoutePaths)
	if err != nil {
		return nil, err
	}

	routes, err := collectRoutes(routeInputs, cfg.SkipPrefixes)
	if err != nil {
		return nil, err
	}

	handlers, registry, err := BuildHandlerIndex(routes, root)
	if err != nil {
		return nil, fmt.Errorf("core: build handler index: %w", err)
	}
	if len(handlers) == 0 {
		return nil, fmt.Errorf("core: no handlers discovered under %s", root)
	}

	projectName := strings.TrimSpace(cfg.ProjectName)
	if projectName == "" {
		projectName = deriveProjectName(root)
	}

	return GenerateOpenAPI(routes, handlers, registry, projectName, cfg.EnableAuthUI)
}

// GenerateAndSaveOpenAPI builds the project OpenAPI document and writes it to disk.
// It returns the absolute path to the generated file alongside the emitted document.
func GenerateAndSaveOpenAPI(configs ...ProjectConfig) (string, []byte, error) {
	spec, err := GenerateProjectOpenAPI(configs...)
	if err != nil {
		return "", nil, err
	}

	var cfg ProjectConfig
	if len(configs) > 0 {
		cfg = configs[0]
	}

	root, err := resolveWorkspaceRoot(cfg.WorkspaceRoot)
	if err != nil {
		return "", nil, err
	}

	output := strings.TrimSpace(cfg.OutputPath)
	if output == "" {
		output = filepath.Join(root, "openapi.json")
	} else if !filepath.IsAbs(output) {
		output = filepath.Join(root, output)
	}
	output = filepath.Clean(output)

	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return "", nil, fmt.Errorf("core: create output dir: %w", err)
	}
	if err := os.WriteFile(output, spec, 0o644); err != nil {
		return "", nil, fmt.Errorf("core: write spec: %w", err)
	}

	return output, spec, nil
}

func resolveWorkspaceRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root != "" {
		if !filepath.IsAbs(root) {
			cwd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("core: determine working directory: %w", err)
			}
			root = filepath.Join(cwd, root)
		}
		return filepath.Clean(root), nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("core: determine working directory: %w", err)
	}

	if moduleRoot, err := FindModuleRoot(cwd); err == nil {
		return moduleRoot, nil
	}

	return filepath.Clean(cwd), nil
}

func resolveRouteInputs(root string, inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return []string{root}, nil
	}

	var paths []string
	seen := make(map[string]struct{})
	for _, p := range inputs {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		abs := trimmed
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, trimmed)
		}
		abs = filepath.Clean(abs)
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		paths = append(paths, abs)
	}

	if len(paths) == 0 {
		return []string{root}, nil
	}

	return paths, nil
}

func collectRoutes(paths []string, skipPrefixes []string) ([]RouteInfo, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("core: no input paths provided")
	}

	skipper := newRouteSkipper(skipPrefixes)
	routeSet := make(map[string]RouteInfo)

	for _, dir := range paths {
		routes, err := FindRoutes(dir)
		if err != nil {
			return nil, fmt.Errorf("core: find routes in %s: %w", dir, err)
		}
		for _, r := range routes {
			if skipper.Skip(r) {
				continue
			}
			key := fmt.Sprintf("%s|%s|%s", filepath.ToSlash(r.File), strings.ToUpper(r.Method), r.Path)
			routeSet[key] = r
		}
	}

	if len(routeSet) == 0 {
		return nil, fmt.Errorf("core: no routes discovered")
	}

	result := make([]RouteInfo, 0, len(routeSet))
	for _, r := range routeSet {
		result = append(result, r)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Path == result[j].Path {
			return result[i].Method < result[j].Method
		}
		return result[i].Path < result[j].Path
	})

	return result, nil
}

func deriveProjectName(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	if modulePath, err := modulePathFromRoot(root); err == nil && strings.TrimSpace(modulePath) != "" {
		modulePath = strings.TrimSpace(modulePath)
		if idx := strings.LastIndex(modulePath, "/"); idx >= 0 && idx < len(modulePath)-1 {
			modulePath = modulePath[idx+1:]
		}
		if modulePath != "" {
			return modulePath
		}
	}
	base := strings.TrimSpace(filepath.Base(root))
	if base != "" && base != "." && base != string(filepath.Separator) {
		return base
	}
	return "Project"
}

type routeSkipper struct {
	prefixes []string
}

func newRouteSkipper(prefixes []string) routeSkipper {
	defaults := []string{"/swagger", "/redoc"}
	var filtered []string
	seen := make(map[string]struct{})
	for _, p := range append(defaults, prefixes...) {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "/") {
			trimmed = "/" + trimmed
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		filtered = append(filtered, trimmed)
	}
	return routeSkipper{prefixes: filtered}
}

func (r routeSkipper) Skip(info RouteInfo) bool {
	path := info.Path
	if path == "" {
		return true
	}
	path = strings.TrimSpace(path)
	for _, prefix := range r.prefixes {
		if strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix)) {
			return true
		}
	}
	if strings.Contains(strings.ToLower(info.File), "/swagger") {
		return true
	}
	return false
}
