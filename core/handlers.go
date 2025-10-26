package core

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// HandlerInfo captures the relevant metadata needed to describe a handler in OpenAPI output.
type HandlerInfo struct {
	ID               string
	Name             string
	Package          string
	File             string
	Receiver         string
	Summary          string
	Description      string
	Notes            []string
	Tags             []string
	Consumes         []string
	Produces         []string
	InputType        string
	OutputType       string
	Responses        map[string]string // HTTP status -> type name (best effort)
	Params           []Parameter
	BodyRequired     bool
	BodyDefined      bool
	FormParams       []Parameter
	ResponseSchemas  map[string]Schema
	NeededComponents []string
	queryParamHints  map[string]*queryParamHint
	EmptyBodyStatus  map[string]bool
	ctxVars          map[string]struct{}
}

// Parameter captures non-body inputs declared via annotations.
type Parameter struct {
	Name        string
	In          string
	Type        string
	Required    bool
	Description string
}

// BuildHandlerIndex groups routes by file and extracts handler metadata.
func BuildHandlerIndex(routes []RouteInfo, workspaceRoot string) (map[string]HandlerInfo, *TypeRegistry, error) {
	local := make(map[string][]RouteInfo)
	external := make(map[string][]RouteInfo)
	for _, r := range routes {
		if r.HandlerID == "" || r.HandlerName == "" {
			continue
		}
		if strings.TrimSpace(r.HandlerImportPath) != "" {
			external[r.HandlerImportPath] = append(external[r.HandlerImportPath], r)
		} else {
			local[r.File] = append(local[r.File], r)
		}
	}

	result := make(map[string]HandlerInfo)
	registry := NewTypeRegistry()
	if workspaceRoot != "" {
		if err := registry.IndexWorkspace(workspaceRoot); err != nil {
			return nil, nil, err
		}
	}

	for file, items := range local {
		infos, err := analyzeHandlersInFile(file, items, registry)
		if err != nil {
			return nil, nil, err
		}
		for id, info := range infos {
			result[id] = info
		}
	}

	if len(external) == 0 {
		return result, registry, nil
	}

	modulePath, err := modulePathFromRoot(workspaceRoot)
	if err != nil {
		return nil, nil, err
	}

	for importPath, items := range external {
		dir, err := resolveImportDir(workspaceRoot, modulePath, importPath)
		if err != nil {
			return nil, nil, err
		}
		infos, err := analyzeHandlersInPackage(importPath, dir, items, registry)
		if err != nil {
			return nil, nil, err
		}
		for id, info := range infos {
			result[id] = info
		}
	}

	return result, registry, nil
}

func analyzeHandlersInFile(filePath string, routes []RouteInfo, registry *TypeRegistry) (map[string]HandlerInfo, error) {
	fset := token.NewFileSet()
	// Parse comments to leverage swagger-like annotations.
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	if registry != nil {
		for _, decl := range node.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				registry.Add(node.Name.Name, filePath, ts)
			}
		}
	}

	needed := make(map[string]RouteInfo)
	for _, r := range routes {
		if r.HandlerID == "" {
			continue
		}
		needed[r.HandlerID] = r
	}

	handlerInfos := make(map[string]HandlerInfo)

	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		id := buildHandlerID(filePath, "", fn.Name.Name)
		route, ok := needed[id]
		if !ok {
			continue
		}

		info := HandlerInfo{
			ID:              route.HandlerID,
			Name:            fn.Name.Name,
			Package:         node.Name.Name,
			File:            filePath,
			Receiver:        extractReceiverType(fn),
			Responses:       make(map[string]string),
			ResponseSchemas: make(map[string]Schema),
			queryParamHints: make(map[string]*queryParamHint),
			EmptyBodyStatus: make(map[string]bool),
			ctxVars:         collectCtxParams(fn),
		}

		populateFromDoc(fn.Doc, &info)
		populateFromBody(fn.Body, &info, registry)
		ensurePathParameters(&info, route)

		// Ensure OutputType mirrors the default success response if defined.
		if info.OutputType == "" {
			if success, ok := info.Responses["200"]; ok {
				info.OutputType = success
			}
		}

		handlerInfos[route.HandlerID] = info
	}

	return handlerInfos, nil
}

func analyzeHandlersInPackage(importPath, dir string, routes []RouteInfo, registry *TypeRegistry) (map[string]HandlerInfo, error) {
	if dir == "" {
		return nil, fmt.Errorf("core: unresolved directory for %s", importPath)
	}
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info fs.FileInfo) bool {
		name := info.Name()
		if strings.HasSuffix(name, "_test.go") {
			return false
		}
		return strings.HasSuffix(name, ".go")
	}, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	needed := make(map[string]RouteInfo)
	for _, r := range routes {
		if r.HandlerID == "" {
			continue
		}
		needed[r.HandlerID] = r
	}

	handlerInfos := make(map[string]HandlerInfo)

	for _, pkg := range pkgs {
		for filePath, node := range pkg.Files {
			if registry != nil {
				for _, decl := range node.Decls {
					gen, ok := decl.(*ast.GenDecl)
					if !ok || gen.Tok != token.TYPE {
						continue
					}
					for _, spec := range gen.Specs {
						ts, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}
						registry.Add(pkg.Name, filePath, ts)
					}
				}
			}
			for _, decl := range node.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name == nil {
					continue
				}
				id := buildHandlerID(filePath, importPath, fn.Name.Name)
				route, ok := needed[id]
				if !ok {
					continue
				}

				info := HandlerInfo{
					ID:              route.HandlerID,
					Name:            fn.Name.Name,
					Package:         pkg.Name,
					File:            filePath,
					Receiver:        extractReceiverType(fn),
					Responses:       make(map[string]string),
					ResponseSchemas: make(map[string]Schema),
					queryParamHints: make(map[string]*queryParamHint),
					EmptyBodyStatus: make(map[string]bool),
					ctxVars:         collectCtxParams(fn),
				}

				populateFromDoc(fn.Doc, &info)
				populateFromBody(fn.Body, &info, registry)
				ensurePathParameters(&info, route)

				if info.OutputType == "" {
					if success, ok := info.Responses["200"]; ok {
						info.OutputType = success
					}
				}

				handlerInfos[route.HandlerID] = info
			}
		}
	}

	return handlerInfos, nil
}

func extractReceiverType(fn *ast.FuncDecl) string {
	if fn == nil || fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	recv := fn.Recv.List[0]
	if recv == nil {
		return ""
	}
	switch t := recv.Type.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok && ident != nil {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func ensurePathParameters(info *HandlerInfo, route RouteInfo) {
	if info == nil {
		return
	}
	params := extractPathParams(route.Path)
	if len(params) == 0 {
		return
	}
	existing := make(map[string]struct{})
	for _, p := range info.Params {
		if strings.EqualFold(p.In, "path") {
			existing[strings.ToLower(p.Name)] = struct{}{}
		}
	}
	for _, name := range params {
		key := strings.ToLower(name)
		if _, ok := existing[key]; ok {
			continue
		}
		info.Params = append(info.Params, Parameter{
			Name:     name,
			In:       "path",
			Required: true,
			Type:     "string",
		})
		existing[key] = struct{}{}
	}
}

func extractPathParams(routePath string) []string {
	routePath = strings.TrimSpace(routePath)
	if routePath == "" {
		return nil
	}
	segments := strings.Split(routePath, "/")
	var params []string
	seen := make(map[string]struct{})
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		if strings.HasPrefix(segment, ":") {
			name := strings.TrimPrefix(segment, ":")
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := seen[strings.ToLower(name)]; ok {
				continue
			}
			seen[strings.ToLower(name)] = struct{}{}
			params = append(params, name)
			continue
		}
		if strings.HasPrefix(segment, "*") {
			name := strings.TrimPrefix(segment, "*")
			name = strings.TrimSpace(name)
			if name == "" {
				name = "wildcard"
			}
			if _, ok := seen[strings.ToLower(name)]; ok {
				continue
			}
			seen[strings.ToLower(name)] = struct{}{}
			params = append(params, name)
		}
	}
	return params
}

func modulePathFromRoot(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("core: workspace root required")
	}
	file, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("core: module path not found in go.mod")
}

func resolveImportDir(root, modulePath, importPath string) (string, error) {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return "", fmt.Errorf("core: empty import path")
	}
	modulePath = strings.TrimSpace(modulePath)
	if modulePath == "" {
		return "", fmt.Errorf("core: module path unknown")
	}
	if !strings.HasPrefix(importPath, modulePath) {
		return "", fmt.Errorf("core: import %s outside module %s", importPath, modulePath)
	}
	rel := strings.TrimPrefix(importPath, modulePath)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return root, nil
	}
	return filepath.Join(root, filepath.FromSlash(rel)), nil
}

func populateFromDoc(doc *ast.CommentGroup, info *HandlerInfo) {
	if doc == nil {
		return
	}

	for _, comment := range doc.List {
		line := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "/*"))
		line = strings.TrimSpace(strings.TrimSuffix(line, "*/"))
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "@") {
			// Treat plain comment lines as description if no explicit description was found.
			if info.Description == "" {
				info.Description = line
			} else {
				info.Description += " " + line
			}
			info.Notes = append(info.Notes, line)
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		tag := fields[0]
		rest := strings.TrimSpace(strings.TrimPrefix(line, tag))
		switch tag {
		case "@Summary":
			info.Summary = strings.TrimSpace(rest)
		case "@Description":
			info.Description = strings.TrimSpace(rest)
		case "@Tags":
			tags := splitCSV(rest)
			info.Tags = appendUnique(info.Tags, tags...)
		case "@Accept":
			info.Consumes = appendUnique(info.Consumes, splitCSV(rest)...)
		case "@Produce":
			info.Produces = appendUnique(info.Produces, splitCSV(rest)...)
		case "@Param":
			parseParamAnnotation(rest, info)
		case "@Success", "@Failure":
			parseResponseAnnotation(fields, info)
		}
	}
}

func populateFromBody(body *ast.BlockStmt, info *HandlerInfo, registry *TypeRegistry) {
	if body == nil {
		return
	}

	varTypes := make(map[string]string)
	for _, param := range info.Params {
		if param.Name != "" && param.Type != "" {
			varTypes[param.Name] = param.Type
		}
	}
	for _, param := range info.FormParams {
		if param.Name != "" && param.Type != "" {
			varTypes[param.Name] = param.Type
		}
	}

	queryBindings := make(map[string]string)

	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.DeclStmt:
			decl, ok := node.Decl.(*ast.GenDecl)
			if !ok || decl.Tok != token.VAR {
				return true
			}
			for _, spec := range decl.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				var typeStr string
				if vs.Type != nil {
					typeStr = exprToString(vs.Type)
				}
				for i, name := range vs.Names {
					if name == nil {
						continue
					}
					if typeStr == "" && i < len(vs.Values) {
						typeStr = inferTypeFromExpr(vs.Values[i], registry)
					}
					if typeStr != "" {
						varTypes[name.Name] = typeStr
					}
				}
			}
		case *ast.AssignStmt:
			if node.Tok != token.DEFINE {
				// track direct assignments (e.g. t = append(...))
				handleAssignmentForQuery(node, info, varTypes, queryBindings)
				return true
			}
			if len(node.Rhs) == 1 {
				if call, ok := node.Rhs[0].(*ast.CallExpr); ok {
					results := lookupFunctionReturnTypes(registry, call, len(node.Lhs))
					for i, lhs := range node.Lhs {
						ident, ok := lhs.(*ast.Ident)
						if !ok || ident.Name == "_" {
							continue
						}
						var typ string
						if len(results) > i {
							typ = results[i]
						}
						if typ == "" {
							typ = inferTypeFromExpr(call, registry)
						}
						if typ != "" {
							varTypes[ident.Name] = typ
						}
					}
					names := collectQueryParamNames(call, queryBindings)
					bindQueryNames(node.Lhs, names, queryBindings)
					for _, name := range names {
						ensureQueryParam(info, name, false)
					}
					handleCallForQueryHints(call, node.Lhs, info, queryBindings)
					return true
				}
			}
			for i, lhs := range node.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || ident.Name == "_" {
					continue
				}
				if i >= len(node.Rhs) {
					continue
				}
				if typ := inferTypeFromExpr(node.Rhs[i], registry); typ != "" {
					varTypes[ident.Name] = typ
				}
			}
			handleAssignmentForQuery(node, info, varTypes, queryBindings)
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil {
				return true
			}
			switch sel.Sel.Name {
			case "BodyParser":
				if info.InputType != "" || len(node.Args) == 0 {
					return true
				}
				if unary, ok := node.Args[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
					if ident, ok := unary.X.(*ast.Ident); ok {
						if typ, ok := varTypes[ident.Name]; ok {
							info.InputType = typ
						}
					}
				}
			case "FormFile":
				if len(node.Args) == 0 {
					return true
				}
				if lit, ok := node.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if name, err := strconv.Unquote(lit.Value); err == nil {
						ensureFormParam(info, Parameter{
							Name:     name,
							In:       "formData",
							Type:     "file",
							Required: true,
						})
					}
				}
			case "FormValue":
				if len(node.Args) == 0 {
					return true
				}
				if lit, ok := node.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if name, err := strconv.Unquote(lit.Value); err == nil {
						ensureFormParam(info, Parameter{
							Name:        name,
							In:          "formData",
							Type:        "string",
							Required:    false,
							Description: "",
						})
					}
				}
			case "Query":
				if len(node.Args) == 0 {
					return true
				}
				if lit, ok := node.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if name, err := strconv.Unquote(lit.Value); err == nil {
						ensureQueryParam(info, name, false)
					}
				}
			case "QueryInt", "QueryBool", "QueryFloat":
				if len(node.Args) == 0 {
					return true
				}
				if lit, ok := node.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if name, err := strconv.Unquote(lit.Value); err == nil {
						ensureQueryParam(info, name, false)
					}
				}
			case "QueryParser":
				if len(node.Args) == 0 {
					return true
				}
				typeName := strings.TrimSpace(inferTypeFromExpr(node.Args[0], registry))
				if direct := strings.TrimSpace(varTypes[typeName]); direct != "" {
					typeName = direct
				}
				if typeName == "" {
					if unary, ok := node.Args[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
						if ident, ok := unary.X.(*ast.Ident); ok {
							typeName = varTypes[ident.Name]
						}
					}
				}
				ensureQueryStructParams(info, typeName, registry)
			}
			if processCtxResponseCall(node, info, varTypes, registry) {
				return true
			}
		case *ast.ReturnStmt:
			handleReturnResponses(node, info, varTypes, registry)
		}
		return true
	})
}

func handleReturnResponses(ret *ast.ReturnStmt, info *HandlerInfo, varTypes map[string]string, registry *TypeRegistry) {
	if ret == nil || info == nil {
		return
	}
	for _, result := range ret.Results {
		call, ok := result.(*ast.CallExpr)
		if !ok {
			continue
		}
		if processCtxResponseCall(call, info, varTypes, registry) {
			continue
		}
		if handleResponseHelper(call, info, varTypes, registry) {
			continue
		}
	}
}

func handleResponseHelper(call *ast.CallExpr, info *HandlerInfo, varTypes map[string]string, registry *TypeRegistry) bool {
	if call == nil || info == nil {
		return false
	}
	if processCtxResponseCall(call, info, varTypes, registry) {
		return true
	}
	name := functionName(call)
	if name == "" {
		return false
	}
	switch name {
	case "OKResult":
		if len(call.Args) < 2 {
			return false
		}
		addResponseFromExpr(info, "200", call.Args[1], varTypes, registry)
		return true
	case "JSON":
		if len(call.Args) < 3 {
			return false
		}
		status := normalizeStatusLiteral(call.Args[1])
		if status == "" {
			status = "200"
		}
		addResponseFromExpr(info, status, call.Args[2], varTypes, registry)
		return true
	case "BadRequest":
		ensureErrorResponse(info, "400")
		return true
	case "NotFound":
		ensureErrorResponse(info, "404")
		return true
	case "InternalError":
		ensureErrorResponse(info, "500")
		return true
	default:
		return false
	}
}

func handleAssignmentForQuery(assign *ast.AssignStmt, info *HandlerInfo, varTypes map[string]string, bindings map[string]string) {
	if assign == nil || info == nil {
		return
	}
	if len(assign.Rhs) != 1 {
		return
	}
	switch rhs := assign.Rhs[0].(type) {
	case *ast.CallExpr:
		names := collectQueryParamNames(rhs, bindings)
		bindQueryNames(assign.Lhs, names, bindings)
		for _, name := range names {
			ensureQueryParam(info, name, false)
		}
		handleCallForQueryHints(rhs, assign.Lhs, info, bindings)
	case *ast.Ident:
		if name, ok := bindings[rhs.Name]; ok {
			bindQueryNames(assign.Lhs, []string{name}, bindings)
		}
	}
}

func handleCallForQueryHints(call *ast.CallExpr, lhs []ast.Expr, info *HandlerInfo, bindings map[string]string) {
	if call == nil || info == nil {
		return
	}
	if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "append" {
		for idx, arg := range call.Args {
			if idx == 0 {
				continue
			}
			names := collectQueryParamNames(arg, bindings)
			for _, name := range names {
				ensureQueryParam(info, name, true)
			}
		}
		return
	}
	names := collectQueryParamNames(call, bindings)
	for _, name := range names {
		ensureQueryParam(info, name, false)
	}
}

func ensureErrorResponse(info *HandlerInfo, status string) {
	if info == nil || status == "" {
		return
	}
	if info.Responses == nil {
		info.Responses = make(map[string]string)
	}
	if _, exists := info.Responses[status]; !exists {
		info.Responses[status] = "fiber.Map"
	}
	if info.ResponseSchemas == nil {
		info.ResponseSchemas = make(map[string]Schema)
	}
	if _, exists := info.ResponseSchemas[status]; !exists {
		info.ResponseSchemas[status] = Schema{
			"type": "object",
			"properties": map[string]interface{}{
				"error": map[string]interface{}{"type": "string"},
			},
			"required": []string{"error"},
		}
	}
}

func ensureEmptyResponse(info *HandlerInfo, status string) {
	if info == nil || status == "" {
		return
	}
	if info.EmptyBodyStatus == nil {
		info.EmptyBodyStatus = make(map[string]bool)
	}
	info.EmptyBodyStatus[status] = true
}

func ensureBinaryResponse(info *HandlerInfo, status string) {
	if info == nil || status == "" {
		return
	}
	if info.ResponseSchemas == nil {
		info.ResponseSchemas = make(map[string]Schema)
	}
	if _, exists := info.ResponseSchemas[status]; !exists {
		info.ResponseSchemas[status] = Schema{
			"type":   "string",
			"format": "binary",
		}
	}
}

func ensureTextResponse(info *HandlerInfo, status string) {
	if info == nil || status == "" {
		return
	}
	if info.ResponseSchemas == nil {
		info.ResponseSchemas = make(map[string]Schema)
	}
	if _, exists := info.ResponseSchemas[status]; !exists {
		info.ResponseSchemas[status] = Schema{
			"type": "string",
		}
	}
}

func addResponseFromExpr(info *HandlerInfo, status string, expr ast.Expr, varTypes map[string]string, registry *TypeRegistry) {
	if info == nil || expr == nil || status == "" {
		return
	}
	responseType := inferTypeFromExpr(expr, registry)
	if ident, ok := expr.(*ast.Ident); ok {
		if inferred := varTypes[ident.Name]; inferred != "" {
			responseType = inferred
		} else if responseType == "" {
			responseType = ident.Name
		}
	}
	if comp, ok := expr.(*ast.CompositeLit); ok {
		if schema := schemaFromCompositeLiteral(comp, varTypes, registry, info); schema != nil {
			if info.ResponseSchemas == nil {
				info.ResponseSchemas = make(map[string]Schema)
			}
			if _, exists := info.ResponseSchemas[status]; !exists {
				info.ResponseSchemas[status] = schema
			}
		}
	}
	if responseType == "" {
		return
	}
	if info.Responses == nil {
		info.Responses = make(map[string]string)
	}
	if _, exists := info.Responses[status]; !exists {
		info.Responses[status] = responseType
	}
	if status == "200" && info.OutputType == "" {
		info.OutputType = responseType
	}
}

func ensureFormParam(info *HandlerInfo, param Parameter) {
	if info == nil || strings.TrimSpace(param.Name) == "" {
		return
	}
	param.In = "formData"
	if strings.TrimSpace(param.Type) == "" {
		param.Type = "string"
	}
	key := strings.ToLower(param.Name)
	for _, existing := range info.FormParams {
		if strings.ToLower(existing.Name) == key {
			return
		}
	}
	info.FormParams = append(info.FormParams, param)
}

type queryParamHint struct {
	allowMultiple bool
}

func ensureQueryParam(info *HandlerInfo, name string, allowMultiple bool) {
	if info == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	key := strings.ToLower(name)
	if info.queryParamHints == nil {
		info.queryParamHints = make(map[string]*queryParamHint)
	}
	hint := info.queryParamHints[key]
	if hint == nil {
		hint = &queryParamHint{}
		info.queryParamHints[key] = hint
	}
	if allowMultiple {
		hint.allowMultiple = true
	}
	paramType := "string"
	if hint.allowMultiple {
		paramType = "[]string"
	}
	for i := range info.Params {
		if strings.EqualFold(info.Params[i].In, "query") && strings.ToLower(info.Params[i].Name) == key {
			if hint.allowMultiple && !strings.HasPrefix(info.Params[i].Type, "[]") {
				info.Params[i].Type = "[]string"
			}
			return
		}
	}
	info.Params = append(info.Params, Parameter{
		Name:     name,
		In:       "query",
		Type:     paramType,
		Required: false,
	})
}

type ctxResponseKind int

const (
	ctxResponseUnknown ctxResponseKind = iota
	ctxResponseJSON
	ctxResponseEmpty
	ctxResponseBinary
	ctxResponseText
)

func processCtxResponseCall(call *ast.CallExpr, info *HandlerInfo, varTypes map[string]string, registry *TypeRegistry) bool {
	if call == nil || info == nil {
		return false
	}
	kind, status, body := classifyCtxResponseCall(call, info)
	if kind == ctxResponseUnknown {
		return false
	}
	if status == "" {
		status = "200"
	}
	switch kind {
	case ctxResponseJSON:
		if body == nil {
			return false
		}
		addResponseFromExpr(info, status, body, varTypes, registry)
	case ctxResponseEmpty:
		ensureEmptyResponse(info, status)
	case ctxResponseBinary:
		ensureBinaryResponse(info, status)
		info.Produces = appendUnique(info.Produces, "application/octet-stream")
	case ctxResponseText:
		ensureTextResponse(info, status)
		info.Produces = appendUnique(info.Produces, "text/plain")
	default:
		return false
	}
	return true
}

func classifyCtxResponseCall(call *ast.CallExpr, info *HandlerInfo) (ctxResponseKind, string, ast.Expr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return ctxResponseUnknown, "", nil
	}
	method := sel.Sel.Name
	status, ok := unwrapCtxReceiver(sel.X, info)
	if !ok {
		return ctxResponseUnknown, "", nil
	}
	switch method {
	case "JSON":
		if len(call.Args) == 0 {
			return ctxResponseUnknown, "", nil
		}
		return ctxResponseJSON, status, call.Args[0]
	case "SendStatus":
		if len(call.Args) > 0 {
			if normalized := normalizeStatusLiteral(call.Args[0]); normalized != "" {
				status = normalized
			}
		}
		return ctxResponseEmpty, status, nil
	case "SendFile", "SendStream", "Download":
		return ctxResponseBinary, status, nil
	case "SendString":
		return ctxResponseText, status, callArgOrNil(call, 0)
	case "Redirect":
		if len(call.Args) > 1 {
			if normalized := normalizeStatusLiteral(call.Args[1]); normalized != "" {
				status = normalized
			}
		}
		return ctxResponseEmpty, status, nil
	default:
		return ctxResponseUnknown, "", nil
	}
}

func callArgOrNil(call *ast.CallExpr, idx int) ast.Expr {
	if call == nil || idx < 0 || idx >= len(call.Args) {
		return nil
	}
	return call.Args[idx]
}

func unwrapCtxReceiver(expr ast.Expr, info *HandlerInfo) (string, bool) {
	switch v := expr.(type) {
	case *ast.CallExpr:
		sel, ok := v.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil {
			return "", false
		}
		if sel.Sel.Name == "Status" {
			status := "200"
			if len(v.Args) > 0 {
				if normalized := normalizeStatusLiteral(v.Args[0]); normalized != "" {
					status = normalized
				}
			}
			if _, ok := unwrapCtxReceiver(sel.X, info); !ok {
				return "", false
			}
			return status, true
		}
		return "", false
	case *ast.Ident:
		if info.ctxVars == nil {
			return "", false
		}
		_, ok := info.ctxVars[v.Name]
		return "", ok
	case *ast.StarExpr:
		return unwrapCtxReceiver(v.X, info)
	case *ast.SelectorExpr:
		if ident, ok := v.X.(*ast.Ident); ok && info.ctxVars != nil {
			if _, exists := info.ctxVars[ident.Name]; exists {
				return "", true
			}
		}
		return "", false
	default:
		return "", false
	}
}

func collectQueryParamNames(expr ast.Expr, bindings map[string]string) []string {
	if expr == nil {
		return nil
	}
	var names []string
	switch v := expr.(type) {
	case *ast.CallExpr:
		if name, ok := extractQueryParamName(v); ok {
			names = append(names, name)
		}
		for _, arg := range v.Args {
			names = append(names, collectQueryParamNames(arg, bindings)...)
		}
	case *ast.Ident:
		if name, ok := bindings[v.Name]; ok {
			names = append(names, name)
		}
	}
	return names
}

func bindQueryNames(lhs []ast.Expr, names []string, bindings map[string]string) {
	if len(names) == 0 || len(lhs) == 0 {
		return
	}
	selected := names[0]
	for _, target := range lhs {
		if ident, ok := target.(*ast.Ident); ok && ident.Name != "_" {
			if bindings != nil {
				bindings[ident.Name] = selected
			}
		}
	}
}

func extractQueryParamName(call *ast.CallExpr) (string, bool) {
	if call == nil {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return "", false
	}
	if sel.Sel.Name != "Query" {
		return "", false
	}
	if len(call.Args) == 0 {
		return "", false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	name, err := strconv.Unquote(lit.Value)
	if err != nil {
		name = strings.Trim(lit.Value, "\"")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	return name, true
}

func collectCtxParams(fn *ast.FuncDecl) map[string]struct{} {
	result := make(map[string]struct{})
	if fn == nil || fn.Type == nil || fn.Type.Params == nil {
		return result
	}
	for _, field := range fn.Type.Params.List {
		if field == nil || len(field.Names) == 0 {
			continue
		}
		typeName := strings.TrimSpace(exprToString(field.Type))
		if !isFiberCtxType(typeName) {
			continue
		}
		for _, name := range field.Names {
			if name == nil || name.Name == "" {
				continue
			}
			result[name.Name] = struct{}{}
		}
	}
	return result
}

func isFiberCtxType(typeName string) bool {
	if typeName == "" {
		return false
	}
	trimmed := strings.TrimPrefix(typeName, "*")
	if trimmed == "fiber.Ctx" || trimmed == "fiberctx.Ctx" {
		return true
	}
	if strings.Contains(strings.ToLower(trimmed), "fiber") && strings.HasSuffix(trimmed, ".Ctx") {
		return true
	}
	if trimmed == "Ctx" {
		return true
	}
	return false
}

func ensureQueryStructParams(info *HandlerInfo, typeName string, registry *TypeRegistry) {
	if info == nil {
		return
	}
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return
	}
	for strings.HasPrefix(typeName, "*") {
		typeName = strings.TrimPrefix(typeName, "*")
	}
	if registry == nil {
		return
	}
	spec, _ := registry.Resolve(typeName, info.Package)
	if spec == nil || spec.Spec == nil {
		return
	}
	structType, ok := spec.Spec.Type.(*ast.StructType)
	if !ok || structType.Fields == nil {
		return
	}
	for _, field := range structType.Fields.List {
		names, allowMany := queryFieldNames(field)
		if len(names) == 0 {
			continue
		}
		for _, name := range names {
			ensureQueryParam(info, name, allowMany)
		}
	}
}

func queryFieldNames(field *ast.Field) ([]string, bool) {
	if field == nil {
		return nil, false
	}
	typeExpr := field.Type
	isSlice := isSliceType(typeExpr)

	nameFromTag := extractTag(field, "query")
	if nameFromTag == "-" {
		return nil, false
	}
	if nameFromTag != "" {
		parts := strings.Split(nameFromTag, ",")
		name := strings.TrimSpace(parts[0])
		if name == "" && len(field.Names) > 0 && field.Names[0] != nil {
			name = strings.TrimSpace(field.Names[0].Name)
		}
		if name == "" {
			return nil, false
		}
		return []string{name}, isSlice
	}
	if len(field.Names) == 0 {
		return nil, false
	}
	var names []string
	for _, ident := range field.Names {
		if ident == nil || ident.Name == "" {
			continue
		}
		names = append(names, ident.Name)
	}
	return names, isSlice
}

func extractTag(field *ast.Field, key string) string {
	if field == nil || field.Tag == nil {
		return ""
	}
	raw, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		raw = strings.Trim(field.Tag.Value, "`")
	}
	tag := reflect.StructTag(raw)
	return tag.Get(key)
}

func isSliceType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.ArrayType:
		return t.Len == nil
	case *ast.StarExpr:
		return isSliceType(t.X)
	case *ast.Ident:
		return strings.HasPrefix(strings.ToLower(t.Name), "[]")
	default:
		return false
	}
}

func parseResponseAnnotation(fields []string, info *HandlerInfo) {
	if len(fields) < 4 {
		return
	}
	status := fields[1]
	// swagger annotations often include {object} token at index 2.
	typeIdx := 2
	if fields[2] == "{object}" && len(fields) >= 4 {
		typeIdx = 3
	}
	if typeIdx >= len(fields) {
		return
	}
	typ := cleanTypeToken(fields[typeIdx])
	if typ == "" {
		return
	}
	if info.Responses == nil {
		info.Responses = make(map[string]string)
	}
	info.Responses[status] = typ
	if status == "200" && info.OutputType == "" {
		info.OutputType = typ
	}
}

func parseParamAnnotation(rest string, info *HandlerInfo) {
	parts := splitAnnotationFields(rest)
	if len(parts) < 3 {
		return
	}
	name := parts[0]
	location := canonicalParamLocation(parts[1])
	typeToken := cleanTypeToken(parts[2])
	required := false
	if len(parts) >= 4 {
		required = parseBoolToken(parts[3])
	}
	description := ""
	if len(parts) >= 5 {
		description = strings.Join(parts[4:], " ")
	}

	switch location {
	case "body":
		if info.InputType == "" {
			info.InputType = typeToken
		}
		if !info.BodyRequired {
			info.BodyRequired = required
		}
		info.BodyDefined = true
		return
	case "path":
		required = true
	case "formData":
		info.FormParams = append(info.FormParams, Parameter{
			Name:        name,
			In:          location,
			Type:        typeToken,
			Required:    required,
			Description: description,
		})
		return
	}

	info.Params = append(info.Params, Parameter{
		Name:        name,
		In:          location,
		Type:        typeToken,
		Required:    required,
		Description: description,
	})
}

func inferTypeFromExpr(expr ast.Expr, registry *TypeRegistry) string {
	switch v := expr.(type) {
	case *ast.CompositeLit:
		return exprToString(v.Type)
	case *ast.CallExpr:
		if res := lookupFunctionReturnTypes(registry, v, 1); len(res) == 1 {
			return res[0]
		}
		if ident, ok := v.Fun.(*ast.Ident); ok {
			switch ident.Name {
			case "new", "make":
				if len(v.Args) > 0 {
					return exprToString(v.Args[0])
				}
			}
		}
	case *ast.UnaryExpr:
		if v.Op == token.AND {
			return inferTypeFromExpr(v.X, registry)
		}
	case *ast.Ident:
		return v.Name
	}
	return ""
}

func lookupFunctionReturnTypes(registry *TypeRegistry, call *ast.CallExpr, expected int) []string {
	if registry == nil || call == nil {
		return nil
	}
	name := functionName(call)
	if name == "" {
		return nil
	}
	candidates := registry.functions[name]
	if len(candidates) == 0 {
		return nil
	}
	alias := ""
	if expr := strings.TrimSpace(exprToString(call.Fun)); expr != "" {
		if idx := strings.LastIndex(expr, "."); idx > 0 {
			alias = strings.TrimSpace(expr[:idx])
		}
	}
	filtered := candidates
	if alias != "" {
		alias = strings.TrimPrefix(alias, "*")
		aliasLower := strings.ToLower(alias)
		var tmp []FuncSignature
		for _, cand := range candidates {
			if strings.ToLower(cand.Package) == aliasLower {
				tmp = append(tmp, cand)
			}
		}
		if len(tmp) > 0 {
			filtered = tmp
		}
	}
	if len(filtered) == 1 {
		if expected == 0 || len(filtered[0].Results) == expected {
			return filtered[0].Results
		}
		return nil
	}
	var match []string
	for _, cand := range filtered {
		if expected > 0 && len(cand.Results) != expected {
			continue
		}
		if match != nil {
			return nil
		}
		match = cand.Results
	}
	return match
}

func functionName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		if fn.Sel != nil {
			return fn.Sel.Name
		}
	}
	return ""
}

func normalizeStatusLiteral(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.BasicLit:
		return strings.Trim(v.Value, "\"")
	case *ast.SelectorExpr:
		rendered := exprToString(v)
		if code, ok := httpStatusLookup[rendered]; ok {
			return code
		}
		return rendered
	case *ast.Ident:
		return v.Name
	default:
		return exprToString(v)
	}
}

var httpStatusLookup = map[string]string{
	"http.StatusOK":                  "200",
	"http.StatusCreated":             "201",
	"http.StatusAccepted":            "202",
	"http.StatusNoContent":           "204",
	"http.StatusBadRequest":          "400",
	"http.StatusUnauthorized":        "401",
	"http.StatusForbidden":           "403",
	"http.StatusNotFound":            "404",
	"http.StatusNotImplemented":      "501",
	"http.StatusConflict":            "409",
	"http.StatusInternalServerError": "500",
	"http.StatusServiceUnavailable":  "503",
}

func splitAnnotationFields(input string) []string {
	if input == "" {
		return nil
	}
	var (
		result  []string
		current strings.Builder
		inQuote bool
		escape  bool
	)
	for _, r := range input {
		switch {
		case escape:
			current.WriteRune(r)
			escape = false
		case r == '\\' && inQuote:
			escape = true
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}

func parseBoolToken(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "required":
		return true
	default:
		return false
	}
}

func canonicalParamLocation(value string) string {
	v := strings.TrimSpace(strings.ToLower(value))
	switch v {
	case "path", "query", "header", "cookie", "body":
		return v
	case "formdata", "form":
		return "formData"
	default:
		return v
	}
}

func schemaFromCompositeLiteral(comp *ast.CompositeLit, varTypes map[string]string, registry *TypeRegistry, info *HandlerInfo) Schema {
	if comp == nil {
		return nil
	}
	switch comp.Type.(type) {
	case *ast.ArrayType:
		return schemaFromArrayLiteral(comp, varTypes, registry, info)
	case *ast.MapType:
		return schemaFromMapLiteral(comp, varTypes, registry, info)
	case *ast.SelectorExpr, *ast.Ident:
		typeName := exprToString(comp.Type)
		if strings.Contains(typeName, "Map") || strings.HasPrefix(typeName, "map[") {
			return schemaFromMapLiteral(comp, varTypes, registry, info)
		}
	}
	if len(comp.Elts) > 0 {
		if _, ok := comp.Elts[0].(*ast.KeyValueExpr); ok {
			return schemaFromMapLiteral(comp, varTypes, registry, info)
		}
		itemSchema := schemaForValueExpr(comp.Elts[0], varTypes, registry, info)
		if itemSchema == nil {
			itemSchema = Schema{"type": "object"}
		}
		return Schema{
			"type":  "array",
			"items": itemSchema,
		}
	}
	return nil
}

func schemaFromArrayLiteral(comp *ast.CompositeLit, varTypes map[string]string, registry *TypeRegistry, info *HandlerInfo) Schema {
	arrayType, _ := comp.Type.(*ast.ArrayType)
	var itemSchema Schema
	if len(comp.Elts) > 0 {
		itemSchema = schemaForValueExpr(comp.Elts[0], varTypes, registry, info)
	}
	if itemSchema == nil && arrayType != nil {
		itemSchema = schemaFromTypeString(exprToString(arrayType.Elt), info)
	}
	if itemSchema == nil {
		itemSchema = Schema{"type": "object"}
	}
	return Schema{
		"type":  "array",
		"items": itemSchema,
	}
}

func schemaFromMapLiteral(comp *ast.CompositeLit, varTypes map[string]string, registry *TypeRegistry, info *HandlerInfo) Schema {
	props := make(map[string]interface{})
	var required []string
	for _, elt := range comp.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key := literalKeyToString(kv.Key)
		if key == "" {
			continue
		}
		valSchema := schemaForValueExpr(kv.Value, varTypes, registry, info)
		if valSchema == nil {
			valSchema = Schema{"type": "string"}
		}
		props[key] = valSchema
		required = append(required, key)
	}
	schema := Schema{"type": "object"}
	if len(props) > 0 {
		schema["properties"] = props
		schema["additionalProperties"] = false
	}
	if len(required) > 0 {
		sort.Strings(required)
		schema["required"] = required
	}
	return schema
}

func literalKeyToString(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			if s, err := strconv.Unquote(v.Value); err == nil {
				return s
			}
			return strings.Trim(v.Value, "\"")
		}
	case *ast.Ident:
		return v.Name
	}
	return ""
}

func schemaForValueExpr(expr ast.Expr, varTypes map[string]string, registry *TypeRegistry, info *HandlerInfo) Schema {
	switch v := expr.(type) {
	case *ast.BasicLit:
		switch v.Kind {
		case token.STRING:
			return Schema{"type": "string"}
		case token.INT, token.IMAG, token.CHAR:
			return Schema{"type": "integer"}
		case token.FLOAT:
			return Schema{"type": "number"}
		}
	case *ast.CompositeLit:
		return schemaFromCompositeLiteral(v, varTypes, registry, info)
	case *ast.UnaryExpr:
		return schemaForValueExpr(v.X, varTypes, registry, info)
	case *ast.Ident:
		name := strings.ToLower(v.Name)
		if name == "true" || name == "false" {
			return Schema{"type": "boolean"}
		}
		if typeName := varTypes[v.Name]; typeName != "" {
			return schemaFromTypeString(typeName, info)
		}
		return Schema{"type": "string"}
	case *ast.SelectorExpr:
		typeName := exprToString(v)
		if typeName != "" {
			return schemaFromTypeString(typeName, info)
		}
	case *ast.CallExpr:
		if typeName := inferTypeFromExpr(v, registry); typeName != "" {
			return schemaFromTypeString(typeName, info)
		}
	}
	return Schema{"type": "string"}
}

func schemaFromTypeString(typeName string, info *HandlerInfo) Schema {
	if typeName == "" {
		return Schema{"type": "string"}
	}
	t := strings.TrimSpace(typeName)
	for strings.HasPrefix(t, "*") {
		t = strings.TrimPrefix(t, "*")
	}
	if strings.HasPrefix(t, "[]") {
		items := schemaFromTypeString(t[2:], info)
		return Schema{
			"type":  "array",
			"items": items,
		}
	}
	lower := strings.ToLower(t)
	switch lower {
	case "string":
		return Schema{"type": "string"}
	case "bool", "boolean":
		return Schema{"type": "boolean"}
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return Schema{"type": "integer"}
	case "float32", "float64":
		return Schema{"type": "number"}
	case "time.time":
		return Schema{"type": "string", "format": "date-time"}
	case "[]byte":
		return Schema{"type": "string", "format": "byte"}
	}
	component := buildComponentName(t)
	if component != "" {
		if info != nil {
			recordNeededComponent(info, t)
		}
		return Schema{"$ref": fmt.Sprintf("#/components/schemas/%s", component)}
	}
	return Schema{"type": "object"}
}

func recordNeededComponent(info *HandlerInfo, typeName string) {
	if info == nil || typeName == "" {
		return
	}
	key := strings.TrimSpace(typeName)
	for _, existing := range info.NeededComponents {
		if existing == key {
			return
		}
	}
	info.NeededComponents = append(info.NeededComponents, key)
}

func appendUnique(dst []string, values ...string) []string {
	if len(values) == 0 {
		return dst
	}
	existing := make(map[string]struct{}, len(dst))
	for _, v := range dst {
		existing[strings.ToLower(v)] = struct{}{}
	}
	for _, v := range values {
		val := strings.TrimSpace(v)
		if val == "" {
			continue
		}
		key := strings.ToLower(val)
		if _, ok := existing[key]; ok {
			continue
		}
		existing[key] = struct{}{}
		dst = append(dst, val)
	}
	return dst
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

func cleanTypeToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.Trim(token, "{}")
	token = strings.TrimPrefix(token, "*")
	return token
}

func buildComponentName(typeName string) string {
	cleaned := strings.TrimSpace(typeName)
	cleaned = strings.TrimPrefix(cleaned, "*")
	cleaned = strings.ReplaceAll(cleaned, "[]", "Array")
	cleaned = strings.ReplaceAll(cleaned, ".", "_")
	cleaned = strings.ReplaceAll(cleaned, "{}", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	return cleaned
}
