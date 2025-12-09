package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestEnsurePathParameters(t *testing.T) {
	info := &HandlerInfo{}
	route := RouteInfo{Path: "/series/history/:sourceId"}

	ensurePathParameters(info, route)

	if len(info.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(info.Params))
	}
	param := info.Params[0]
	if param.Name != "sourceId" || param.In != "path" || !param.Required {
		t.Fatalf("unexpected param %+v", param)
	}

	// duplicate should not add
	ensurePathParameters(info, route)
	if len(info.Params) != 1 {
		t.Fatalf("expected no duplicates, got %d", len(info.Params))
	}
}

func TestEnsureQueryParamAllowsMultiple(t *testing.T) {
	info := &HandlerInfo{}

	ensureQueryParam(info, "title", false)
	if got := len(info.Params); got != 1 {
		t.Fatalf("expected 1 param, got %d", got)
	}
	if info.Params[0].Type != "string" {
		t.Fatalf("expected string type, got %q", info.Params[0].Type)
	}

	ensureQueryParam(info, "title", true)
	if info.Params[0].Type != "[]string" {
		t.Fatalf("expected []string after allowing multiple, got %q", info.Params[0].Type)
	}

	// adding another multiple call should keep []string
	ensureQueryParam(info, "title", true)
	if info.Params[0].Type != "[]string" {
		t.Fatalf("expected []string to persist, got %q", info.Params[0].Type)
	}
}

func TestEnsureQueryStructParams(t *testing.T) {
	structType := &ast.StructType{
		Fields: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{{Name: "Tag"}},
					Type:  &ast.Ident{Name: "string"},
					Tag: &ast.BasicLit{
						Kind:  token.STRING,
						Value: "`query:\"tag\"`",
					},
				},
				{
					Names: []*ast.Ident{{Name: "Labels"}},
					Type: &ast.ArrayType{
						Elt: &ast.Ident{Name: "string"},
					},
				},
			},
		},
	}
	typeSpec := &ast.TypeSpec{
		Name: &ast.Ident{Name: "Filter"},
		Type: structType,
	}
	registry := NewTypeRegistry()
	registry.Add("pkg", "file.go", typeSpec)

	info := &HandlerInfo{Package: "pkg", queryParamHints: make(map[string]*queryParamHint)}

	ensureQueryStructParams(info, "Filter", registry)

	if len(info.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(info.Params))
	}
	checkParamType(t, info.Params, "tag", false)
	checkParamType(t, info.Params, "Labels", true)
}

func TestCollectQueryParamNamesAppend(t *testing.T) {
	call := &ast.CallExpr{
		Fun: &ast.Ident{Name: "append"},
		Args: []ast.Expr{
			&ast.Ident{Name: "titles"},
			&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   &ast.Ident{Name: "c"},
					Sel: &ast.Ident{Name: "Query"},
				},
				Args: []ast.Expr{
					&ast.BasicLit{Kind: token.STRING, Value: "\"title\""},
				},
			},
		},
	}

	names := collectQueryParamNames(call, nil)
	if len(names) != 1 || names[0] != "title" {
		t.Fatalf("expected [title], got %#v", names)
	}
}

func TestEnsureEmptyResponse(t *testing.T) {
	info := &HandlerInfo{}
	ensureEmptyResponse(info, "204")
	if !info.EmptyBodyStatus["204"] {
		t.Fatalf("expected 204 to be marked empty")
	}
	responses := buildResponses(*info, nil)
	resp, ok := responses["204"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 204 response map")
	}
	if _, hasContent := resp["content"]; hasContent {
		t.Fatalf("expected no content for 204 response")
	}
}

func TestEnsureBinaryResponse(t *testing.T) {
	info := &HandlerInfo{}
	ensureBinaryResponse(info, "200")
	if schema, ok := info.ResponseSchemas["200"]; !ok {
		t.Fatalf("expected binary schema")
	} else if schema["format"] != "binary" {
		t.Fatalf("expected binary format, got %v", schema["format"])
	}
	responses := buildResponses(*info, nil)
	resp, ok := responses["200"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 200 response map")
	}
	content, ok := resp["content"].(map[string]interface{})
	if !ok || len(content) == 0 {
		t.Fatalf("expected content for binary response")
	}
}

func TestProcessCtxResponseCallJSON(t *testing.T) {
	expr, err := parser.ParseExpr("ctx.Status(201).JSON(body)")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr")
	}
	info := &HandlerInfo{
		Responses:       make(map[string]string),
		ResponseSchemas: make(map[string]Schema),
		ctxVars:         map[string]struct{}{"ctx": {}},
	}
	varTypes := map[string]string{"body": "MyPayload"}
	if !processCtxResponseCall(call, info, varTypes, NewTypeRegistry()) {
		t.Fatalf("expected ctx response detection")
	}
	if got := info.Responses["201"]; got != "MyPayload" {
		t.Fatalf("expected response type MyPayload, got %q", got)
	}
}

func TestProcessCtxResponseCallBinary(t *testing.T) {
	expr, err := parser.ParseExpr("ctx.SendFile(\"file.txt\")")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr")
	}
	info := &HandlerInfo{
		Responses:       make(map[string]string),
		ResponseSchemas: make(map[string]Schema),
		ctxVars:         map[string]struct{}{"ctx": {}},
	}
	if !processCtxResponseCall(call, info, nil, NewTypeRegistry()) {
		t.Fatalf("expected binary ctx response detection")
	}
	if schema := info.ResponseSchemas["200"]; schema["format"] != "binary" {
		t.Fatalf("expected binary schema, got %v", schema)
	}
	if len(info.Produces) == 0 || strings.ToLower(info.Produces[0]) != "application/octet-stream" {
		t.Fatalf("expected octet-stream produce entry, got %#v", info.Produces)
	}
}

func TestProcessCtxResponseCallText(t *testing.T) {
	expr, err := parser.ParseExpr("ctx.SendString(\"hi\")")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	call := expr.(*ast.CallExpr)
	info := &HandlerInfo{
		Responses:       make(map[string]string),
		ResponseSchemas: make(map[string]Schema),
		ctxVars:         map[string]struct{}{"ctx": {}},
	}
	if !processCtxResponseCall(call, info, nil, NewTypeRegistry()) {
		t.Fatalf("expected text response detection")
	}
	if schema := info.ResponseSchemas["200"]; schema["type"] != "string" {
		t.Fatalf("expected string schema, got %v", schema)
	}
	if len(info.Produces) == 0 || strings.ToLower(info.Produces[0]) != "text/plain" {
		t.Fatalf("expected text/plain produce, got %#v", info.Produces)
	}
}

func TestCollectCtxParams(t *testing.T) {
	code := "package x\nfunc handler(ctx *fiber.Ctx, id string){}"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "handler.go", code, 0)
	if err != nil {
		t.Fatalf("parse file: %v", err)
	}
	fn := file.Decls[0].(*ast.FuncDecl)
	vars := collectCtxParams(fn)
	if _, ok := vars["ctx"]; !ok {
		t.Fatalf("expected ctx variable to be detected")
	}
}

func checkParamType(t *testing.T, params []Parameter, name string, expectSlice bool) {
	t.Helper()
	for _, p := range params {
		if strings.EqualFold(p.Name, name) {
			if expectSlice {
				if p.Type != "[]string" {
					t.Fatalf("param %s expected []string, got %q", name, p.Type)
				}
			} else {
				if p.Type != "string" {
					t.Fatalf("param %s expected string, got %q", name, p.Type)
				}
			}
			return
		}
	}
	t.Fatalf("param %s not found", name)
}

func TestInferTypeFromFunctionName(t *testing.T) {
	tests := []struct {
		code     string
		expected string
	}{
		{"FromServiceTagSearchResult()", "TagSearchResult"},
		{"FromServiceLocations(x)", "Locations"},
		{"FromServiceWarehouseLocations()", "WarehouseLocationsResult"},
		{"ToDTO()", "DTO"},
		{"NewUser()", "User"},
		{"New()", ""},
		{"FromService()", "Service"},
		{"SomeRandomFunc()", ""},
		// Selector expressions should preserve package qualifier
		{"httpdto.FromServiceTagSearchResult()", "httpdto.TagSearchResult"},
		{"httpdto.FromServiceWarehouseLocations()", "httpdto.WarehouseLocationsResult"},
		{"pkg.ToDTO(x)", "pkg.DTO"},
		{"service.NewUser()", "service.User"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			expr, err := parser.ParseExpr(tt.code)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			call, ok := expr.(*ast.CallExpr)
			if !ok {
				t.Fatalf("expected CallExpr, got %T", expr)
			}

			result := inferTypeFromFunctionName(call)
			if result != tt.expected {
				t.Errorf("inferTypeFromFunctionName(%q) = %q, want %q", tt.code, result, tt.expected)
			}
		})
	}
}
