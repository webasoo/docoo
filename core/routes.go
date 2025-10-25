package core

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	stdpath "path"
	"path/filepath"
	"strconv"
	"strings"
)

// RouteInfo stores details about a Fiber route discovered in a Register method.
type RouteInfo struct {
	Method            string // HTTP verb, e.g. GET, POST
	Path              string // unquoted route path
	Package           string // Go package name
	File              string // absolute or relative file path where route was declared
	HandlerExpr       string // raw expression passed to router (e.g. h.syncFromUpstream)
	HandlerName       string // extracted function/method identifier (e.g. syncFromUpstream)
	HandlerID         string // stable identifier (file + handler name)
	HandlerImportPath string // fully qualified import path when handler lives in another package
}

var httpVerbs = map[string]struct{}{
	"Connect": {},
	"Delete":  {},
	"Get":     {},
	"Head":    {},
	"Options": {},
	"Patch":   {},
	"Post":    {},
	"Put":     {},
	"Trace":   {},
}

var printerFset = token.NewFileSet()

// FindRoutes walks a file or directory tree and extracts Fiber routes from Register methods.
func FindRoutes(path string) ([]RouteInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return findRoutesInDir(path)
	}

	routes, err := findRoutesInFile(path)
	if err != nil {
		return nil, err
	}
	return routes, nil
}

func findRoutesInDir(root string) ([]RouteInfo, error) {
	var routes []RouteInfo
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fileRoutes, err := findRoutesInFile(path)
		if err != nil {
			return err
		}
		routes = append(routes, fileRoutes...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return routes, nil
}

func findRoutesInFile(path string) ([]RouteInfo, error) {
	fset := token.NewFileSet()
	fileNode, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	prefixes := make(map[string]string)
	handlerBindings := make(map[string]string)
	var routes []RouteInfo
	importAliases := make(map[string]string)
	for _, imp := range fileNode.Imports {
		alias := ""
		if imp.Name != nil {
			alias = strings.TrimSpace(imp.Name.Name)
		}
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if alias == "" {
			alias = stdpath.Base(importPath)
		}
		if alias != "" && alias != "." && alias != "_" {
			importAliases[alias] = importPath
		}
	}

	ast.Inspect(fileNode, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			trackHandlerAssign(handlerBindings, node, importAliases)
			handleGroupAssign(prefixes, node)
		case *ast.ValueSpec:
			trackHandlerValueSpec(handlerBindings, node, importAliases)
			handleGroupValueSpec(prefixes, node)
		case *ast.CallExpr:
			if route, ok := extractRouteFromCall(node, fileNode, path, prefixes, importAliases, handlerBindings); ok {
				routes = append(routes, route)
			}
		}
		return true
	})

	return routes, nil
}

func handleGroupAssign(prefixes map[string]string, stmt *ast.AssignStmt) {
	if len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
		return
	}
	call, ok := stmt.Rhs[0].(*ast.CallExpr)
	if !ok {
		return
	}
	ident, ok := stmt.Lhs[0].(*ast.Ident)
	if !ok || ident.Name == "" {
		return
	}
	if prefix, ok := groupPrefixFromCall(prefixes, call); ok {
		prefixes[ident.Name] = prefix
	}
}

func handleGroupValueSpec(prefixes map[string]string, spec *ast.ValueSpec) {
	if len(spec.Names) != 1 || len(spec.Values) != 1 {
		return
	}
	call, ok := spec.Values[0].(*ast.CallExpr)
	if !ok {
		return
	}
	ident := spec.Names[0]
	if ident == nil || ident.Name == "" {
		return
	}
	if prefix, ok := groupPrefixFromCall(prefixes, call); ok {
		prefixes[ident.Name] = prefix
	}
}

func groupPrefixFromCall(prefixes map[string]string, call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "Group" {
		return "", false
	}
	basePrefix := computePrefix(prefixes, sel.X)
	if len(call.Args) > 0 {
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if val, err := strconv.Unquote(lit.Value); err == nil {
				basePrefix = joinRoutePath(basePrefix, val)
			}
		}
	}
	return basePrefix, true
}

func trackHandlerAssign(bindings map[string]string, stmt *ast.AssignStmt, imports map[string]string) {
	if bindings == nil || len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
		return
	}
	ident, ok := stmt.Lhs[0].(*ast.Ident)
	if !ok || ident.Name == "" {
		return
	}
	importPath := importPathFromExpr(stmt.Rhs[0], imports, bindings)
	if importPath == "" {
		return
	}
	bindings[ident.Name] = importPath
}

func trackHandlerValueSpec(bindings map[string]string, spec *ast.ValueSpec, imports map[string]string) {
	if bindings == nil || len(spec.Names) == 0 {
		return
	}
	for idx, name := range spec.Names {
		if name == nil || name.Name == "" {
			continue
		}
		var value ast.Expr
		switch {
		case len(spec.Values) == len(spec.Names):
			value = spec.Values[idx]
		case len(spec.Values) == 1:
			value = spec.Values[0]
		default:
			continue
		}
		importPath := importPathFromExpr(value, imports, bindings)
		if importPath == "" {
			continue
		}
		bindings[name.Name] = importPath
	}
}

func importPathFromExpr(expr ast.Expr, imports map[string]string, bindings map[string]string) string {
	switch v := expr.(type) {
	case *ast.CallExpr:
		return importPathFromExpr(v.Fun, imports, bindings)
	case *ast.SelectorExpr:
		if ident, ok := v.X.(*ast.Ident); ok {
			if importPath := imports[ident.Name]; importPath != "" {
				return importPath
			}
			if importPath := bindings[ident.Name]; importPath != "" {
				return importPath
			}
		}
	case *ast.UnaryExpr:
		return importPathFromExpr(v.X, imports, bindings)
	case *ast.CompositeLit:
		return importPathFromExpr(v.Type, imports, bindings)
	case *ast.Ident:
		if bindings != nil {
			return bindings[v.Name]
		}
	}
	return ""
}

func extractRouteFromCall(call *ast.CallExpr, file *ast.File, filePath string, prefixes map[string]string, imports, bindings map[string]string) (RouteInfo, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return RouteInfo{}, false
	}

	methodName := sel.Sel.Name
	switch {
	case hasVerb(methodName):
		methodName = methodName
	case hasVerb(strings.Title(methodName)):
		methodName = strings.Title(methodName)
	case hasVerb(strings.ToUpper(methodName)):
		methodName = strings.ToUpper(methodName)
	default:
		return RouteInfo{}, false
	}
	method := strings.ToUpper(methodName)

	if len(call.Args) == 0 {
		return RouteInfo{}, false
	}
	pathLit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || pathLit.Kind != token.STRING {
		return RouteInfo{}, false
	}
	pathValue, err := strconv.Unquote(pathLit.Value)
	if err != nil {
		return RouteInfo{}, false
	}

	handlerExpr, handlerName, handlerImport := handlerInfoFromCall(call, imports, bindings)
	if handlerName == "" {
		return RouteInfo{}, false
	}

	prefix := computePrefix(prefixes, sel.X)
	fullPath := joinRoutePath(prefix, pathValue)

	return RouteInfo{
		Method:            method,
		Path:              fullPath,
		Package:           file.Name.Name,
		File:              filePath,
		HandlerExpr:       handlerExpr,
		HandlerName:       handlerName,
		HandlerImportPath: handlerImport,
		HandlerID:         buildHandlerID(filePath, handlerImport, handlerName),
	}, true
}

func handlerInfoFromCall(call *ast.CallExpr, imports, bindings map[string]string) (string, string, string) {
	if len(call.Args) < 2 {
		return "", "", ""
	}
	switch h := call.Args[1].(type) {
	case *ast.Ident:
		return h.Name, h.Name, ""
	case *ast.SelectorExpr:
		if ident, ok := h.X.(*ast.Ident); ok {
			if importPath := imports[ident.Name]; importPath != "" {
				return exprToString(h), h.Sel.Name, importPath
			}
			if importPath := bindings[ident.Name]; importPath != "" {
				return exprToString(h), h.Sel.Name, importPath
			}
		}
		return exprToString(h), h.Sel.Name, ""
	default:
		return exprToString(h), "", ""
	}
}

func hasVerb(name string) bool {
	_, ok := httpVerbs[name]
	return ok
}

func computePrefix(prefixes map[string]string, expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		if prefixes == nil {
			return ""
		}
		if prefix, ok := prefixes[v.Name]; ok {
			return prefix
		}
		return ""
	case *ast.SelectorExpr:
		return computePrefix(prefixes, v.X)
	case *ast.CallExpr:
		sel, ok := v.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil {
			return ""
		}
		if sel.Sel.Name != "Group" {
			return ""
		}
		base := computePrefix(prefixes, sel.X)
		if len(v.Args) > 0 {
			if lit, ok := v.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if val, err := strconv.Unquote(lit.Value); err == nil {
					return joinRoutePath(base, val)
				}
			}
		}
		return base
	default:
		return ""
	}
}

func buildHandlerID(filePath, importPath, handlerName string) string {
	if handlerName == "" {
		return ""
	}
	if strings.TrimSpace(importPath) != "" {
		return importPath + "::" + handlerName
	}
	rel := filepath.ToSlash(filePath)
	return rel + "::" + handlerName
}

// exprToString renders an AST expression back to source (best-effort).
func exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var sb strings.Builder
	if err := printer.Fprint(&sb, printerFset, expr); err != nil {
		return ""
	}
	return sb.String()
}

func joinRoutePath(prefix, path string) string {
	prefix = strings.TrimSpace(prefix)
	path = strings.TrimSpace(path)

	if prefix == "" {
		if path == "" {
			return "/"
		}
		if strings.HasPrefix(path, "/") {
			return path
		}
		return "/" + path
	}

	if path == "" || path == "/" {
		return prefix
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(prefix, "/") + path
}
