package core

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// OpenAPI models the minimal structure we need to serialise the generated document.
type OpenAPI struct {
	OpenAPI    string                 `json:"openapi"`
	Info       map[string]interface{} `json:"info"`
	Paths      map[string]PathItem    `json:"paths"`
	Components Components             `json:"components,omitempty"`
}

// PathItem represents the operations available on a single path.
type PathItem map[string]Operation

// Operation represents an HTTP operation in OpenAPI.
type Operation map[string]interface{}

// Components wraps reusable schema definitions.
type Components struct {
	Schemas map[string]Schema `json:"schemas,omitempty"`
}

// Schema is a free-form JSON schema definition.
type Schema map[string]interface{}

// GenerateOpenAPI builds an OpenAPI JSON spec from route and handler info.
func GenerateOpenAPI(routes []RouteInfo, handlers map[string]HandlerInfo, types *TypeRegistry) ([]byte, error) {
	if len(routes) == 0 {
		return nil, fmt.Errorf("no routes discovered")
	}

	sortedRoutes := make([]RouteInfo, 0, len(routes))
	sortedRoutes = append(sortedRoutes, routes...)
	sort.Slice(sortedRoutes, func(i, j int) bool {
		if sortedRoutes[i].Path == sortedRoutes[j].Path {
			return sortedRoutes[i].Method < sortedRoutes[j].Method
		}
		return sortedRoutes[i].Path < sortedRoutes[j].Path
	})

	paths := make(map[string]PathItem)
	components := Components{Schemas: make(map[string]Schema)}
	builder := newComponentBuilder(types, components.Schemas)

	for _, route := range sortedRoutes {
		handler, ok := handlers[route.HandlerID]
		if !ok {
			continue
		}

		specPath := normalizeOpenAPIPath(route.Path)
		pathItem := paths[specPath]
		if pathItem == nil {
			pathItem = make(PathItem)
			paths[specPath] = pathItem
		}

		summary := handler.Summary
		if summary == "" {
			if len(handler.Notes) > 0 {
				summary = handler.Notes[0]
			} else {
				summary = handler.Name
			}
		}

		operation := Operation{
			"operationId": fmt.Sprintf("%s.%s", handler.Package, handler.Name),
			"summary":     summary,
		}
		if desc := mergeDescription(handler.Description, handler.Notes); desc != "" {
			operation["description"] = desc
		}

		tags := handler.Tags
		if len(tags) == 0 && handler.Package != "" {
			tags = []string{handler.Package}
		}
		if len(tags) > 0 {
			operation["tags"] = tags
		}

		if params := buildParameters(handler, builder); len(params) > 0 {
			operation["parameters"] = params
		}

		if request := buildRequestBody(handler, builder); request != nil {
			operation["requestBody"] = request
		}

		for _, typeName := range handler.NeededComponents {
			builder.ensureComponent(typeName, handler.Package)
		}

		responses := buildResponses(handler, builder)
		operation["responses"] = responses

		pathItem[strings.ToLower(route.Method)] = operation
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("no routes with handler metadata available")
	}

	doc := OpenAPI{
		OpenAPI: "3.0.0",
		Info: map[string]interface{}{
			"title":   "Auto Generated API",
			"version": "1.0.0",
		},
		Paths:      paths,
		Components: components,
	}

	return json.MarshalIndent(doc, "", "  ")
}

func buildRequestBody(handler HandlerInfo, builder *componentBuilder) map[string]interface{} {
	if reqType := strings.TrimSpace(handler.InputType); reqType != "" {
		contentType := pickFirst(handler.Consumes, "application/json")
		schema := schemaOrRef(reqType, handler.Package, builder)
		required := handler.BodyRequired
		if !handler.BodyDefined {
			required = true
		}
		return map[string]interface{}{
			"required": required,
			"content": map[string]interface{}{
				contentType: map[string]interface{}{
					"schema": schema,
				},
			},
		}
	}

	if len(handler.FormParams) == 0 {
		return nil
	}

	props := make(map[string]interface{})
	var required []string
	for _, param := range handler.FormParams {
		if param.Name == "" {
			continue
		}
		schema := schemaForFormParam(param, handler.Package, builder)
		if schema == nil {
			schema = map[string]interface{}{"type": "string"}
		}
		props[param.Name] = schema
		if param.Required {
			required = append(required, param.Name)
		}
	}

	schema := map[string]interface{}{"type": "object"}
	if len(props) > 0 {
		schema["properties"] = props
		schema["additionalProperties"] = false
	}
	if len(required) > 0 {
		sort.Strings(required)
		schema["required"] = required
	}

	contentType := pickFormContentType(handler)
	return map[string]interface{}{
		"required": len(required) > 0,
		"content": map[string]interface{}{
			contentType: map[string]interface{}{
				"schema": schema,
			},
		},
	}
}

func schemaForFormParam(param Parameter, pkg string, builder *componentBuilder) map[string]interface{} {
	typeName := strings.TrimSpace(param.Type)
	if typeName == "" {
		return map[string]interface{}{"type": "string"}
	}
	switch strings.ToLower(typeName) {
	case "file", "binary":
		return map[string]interface{}{"type": "string", "format": "binary"}
	}
	return schemaOrRef(typeName, pkg, builder)
}

func pickFormContentType(handler HandlerInfo) string {
	defaultCT := "application/x-www-form-urlencoded"
	if hasFileFormParam(handler.FormParams) {
		defaultCT = "multipart/form-data"
	}
	ct := pickFirst(handler.Consumes, defaultCT)
	if ct == "" {
		return defaultCT
	}
	if strings.Contains(ct, "json") {
		return defaultCT
	}
	return ct
}

func hasFileFormParam(params []Parameter) bool {
	for _, p := range params {
		if strings.EqualFold(p.Type, "file") || strings.EqualFold(p.Type, "binary") {
			return true
		}
	}
	return false
}

func buildResponses(handler HandlerInfo, builder *componentBuilder) map[string]interface{} {
	responses := make(map[string]interface{})
	if len(handler.Responses) == 0 && len(handler.EmptyBodyStatus) == 0 && len(handler.ResponseSchemas) == 0 {
		responses["200"] = map[string]interface{}{"description": "Success"}
		return responses
	}

	statusSet := make(map[string]struct{})
	for status := range handler.Responses {
		statusSet[status] = struct{}{}
	}
	for status := range handler.EmptyBodyStatus {
		statusSet[status] = struct{}{}
	}
	for status := range handler.ResponseSchemas {
		statusSet[status] = struct{}{}
	}
	if len(statusSet) == 0 {
		statusSet["200"] = struct{}{}
	}

	statuses := make([]string, 0, len(statusSet))
	for status := range statusSet {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)

	for _, status := range statuses {
		resp := map[string]interface{}{
			"description": statusDescription(status),
		}
		if handler.EmptyBodyStatus != nil && handler.EmptyBodyStatus[status] {
			responses[status] = resp
			continue
		}
		contentType := pickFirst(handler.Produces, "application/json")
		if handler.ResponseSchemas != nil {
			if explicit, ok := handler.ResponseSchemas[status]; ok && explicit != nil {
				resp["content"] = map[string]interface{}{
					contentType: map[string]interface{}{
						"schema": explicit,
					},
				}
				responses[status] = resp
				continue
			}
		}
		typ := strings.TrimSpace(handler.Responses[status])
		schema := schemaOrRef(typ, handler.Package, builder)
		if len(schema) > 0 {
			resp["content"] = map[string]interface{}{
				contentType: map[string]interface{}{
					"schema": schema,
				},
			}
		}
		responses[status] = resp
	}
	return responses
}

func mergeDescription(description string, notes []string) string {
	desc := strings.TrimSpace(description)
	if len(notes) == 0 {
		return desc
	}
	notesText := strings.Join(notes, " ")
	notesText = strings.TrimSpace(notesText)
	if notesText == "" {
		return desc
	}
	if desc == "" {
		return notesText
	}
	// Avoid duplicating note content if already present.
	firstNote := strings.TrimSpace(notes[0])
	if firstNote != "" && strings.Contains(desc, firstNote) {
		return desc
	}
	return notesText + "\n\n" + desc
}

func buildParameters(handler HandlerInfo, builder *componentBuilder) []map[string]interface{} {
	if len(handler.Params) == 0 {
		return nil
	}
	params := make([]map[string]interface{}, 0, len(handler.Params))
	seen := make(map[string]struct{})
	for _, p := range handler.Params {
		if strings.EqualFold(p.In, "formdata") || strings.EqualFold(p.In, "formData") {
			continue
		}
		if p.Name == "" || p.In == "" {
			continue
		}
		key := p.In + ":" + p.Name
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		typeName := strings.TrimSpace(p.Type)
		if typeName == "" {
			typeName = "string"
		}
		schema := schemaOrRef(typeName, handler.Package, builder)
		if schema == nil {
			schema = map[string]interface{}{"type": "string"}
		}

		param := map[string]interface{}{
			"name":     p.Name,
			"in":       p.In,
			"required": p.Required,
			"schema":   schema,
		}
		if p.Description != "" {
			param["description"] = p.Description
		}
		params = append(params, param)
	}
	return params
}

func schemaOrRef(typeName, pkg string, builder *componentBuilder) map[string]interface{} {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return map[string]interface{}{"type": "string"}
	}

	// Dereference pointers eagerly.
	for strings.HasPrefix(typeName, "*") {
		typeName = strings.TrimPrefix(typeName, "*")
		typeName = strings.TrimSpace(typeName)
	}

	trimmed := strings.TrimSpace(typeName)
	if strings.HasPrefix(trimmed, "struct") {
		return schemaFromInlineStruct(trimmed, pkg, builder)
	}

	if strings.HasPrefix(typeName, "[]") {
		items := schemaOrRef(typeName[2:], pkg, builder)
		if items == nil {
			items = map[string]interface{}{"type": "object"}
		}
		return map[string]interface{}{
			"type":  "array",
			"items": items,
		}
	}

	if strings.HasPrefix(typeName, "map[") {
		closing := strings.Index(typeName, "]")
		if closing > 0 && closing+1 < len(typeName) {
			valueType := strings.TrimSpace(typeName[closing+1:])
			valueSchema := schemaOrRef(valueType, pkg, builder)
			if valueSchema == nil {
				valueSchema = map[string]interface{}{"type": "object"}
			}
			return map[string]interface{}{
				"type":                 "object",
				"additionalProperties": valueSchema,
			}
		}
		return map[string]interface{}{"type": "object"}
	}

	lower := strings.ToLower(typeName)
	switch lower {
	case "string":
		return map[string]interface{}{"type": "string"}
	case "bool", "boolean":
		return map[string]interface{}{"type": "boolean"}
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return map[string]interface{}{"type": "integer"}
	case "float32", "float64":
		return map[string]interface{}{"type": "number"}
	case "interface{}", "map[string]interface{}", "fiber.map":
		return map[string]interface{}{"type": "object"}
	case "time.time":
		return map[string]interface{}{"type": "string", "format": "date-time"}
	case "[]byte":
		return map[string]interface{}{"type": "string", "format": "byte"}
	}

	if builder == nil {
		return map[string]interface{}{"type": "object"}
	}

	compName := builder.ensureComponent(typeName, pkg)
	return map[string]interface{}{
		"$ref": fmt.Sprintf("#/components/schemas/%s", compName),
	}
}

func schemaFromInlineStruct(typeExpr, pkg string, builder *componentBuilder) map[string]interface{} {
	expr, err := parser.ParseExpr(typeExpr)
	if err != nil {
		return map[string]interface{}{"type": "object"}
	}
	structType, ok := expr.(*ast.StructType)
	if !ok {
		return map[string]interface{}{"type": "object"}
	}
	return schemaFromStructType(structType, pkg, builder)
}

func schemaFromStructType(structType *ast.StructType, pkg string, builder *componentBuilder) map[string]interface{} {
	if structType == nil {
		return map[string]interface{}{"type": "object"}
	}

	props := make(map[string]interface{})
	var required []string

	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 {
			continue
		}
		for _, name := range field.Names {
			if name == nil || name.Name == "" {
				continue
			}
			meta := extractJSONMetadata(field, name.Name)
			if meta.skip || meta.name == "" {
				continue
			}
			fieldSchema := schemaForStructField(field.Type, pkg, builder)
			if fieldSchema == nil {
				fieldSchema = map[string]interface{}{"type": "string"}
			}
			props[meta.name] = fieldSchema
			optional := meta.omitEmpty || isPointerType(field.Type)
			if !optional {
				required = append(required, meta.name)
			}
		}
	}

	schema := map[string]interface{}{"type": "object"}
	if len(props) > 0 {
		schema["properties"] = props
	}
	if len(required) > 0 {
		sort.Strings(required)
		schema["required"] = required
	}
	return schema
}

func schemaForStructField(expr ast.Expr, pkg string, builder *componentBuilder) map[string]interface{} {
	if expr == nil {
		return map[string]interface{}{"type": "string"}
	}
	return schemaOrRef(exprToString(expr), pkg, builder)
}

func normalizeOpenAPIPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if segment == "" {
			continue
		}
		trimmed := strings.TrimSpace(segment)
		switch {
		case strings.HasPrefix(trimmed, ":"):
			name := strings.TrimPrefix(trimmed, ":")
			name = strings.TrimSpace(name)
			if name == "" {
				name = "param"
			}
			segments[i] = "{" + name + "}"
		case strings.HasPrefix(trimmed, "*"):
			name := strings.TrimPrefix(trimmed, "*")
			name = strings.TrimSpace(name)
			if name == "" {
				name = "wildcard"
			}
			segments[i] = "{" + name + "}"
		default:
			segments[i] = trimmed
		}
	}
	result := strings.Join(segments, "/")
	if !strings.HasPrefix(result, "/") {
		result = "/" + result
	}
	return result
}

func pickFirst(values []string, fallback string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return canonicalContentType(strings.TrimSpace(v), fallback)
		}
	}
	return canonicalContentType("", fallback)
}

func statusDescription(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "Response"
	}
	if desc, ok := statusDescriptions[status]; ok {
		return desc
	}
	if code, err := strconv.Atoi(status); err == nil {
		if text := http.StatusText(code); text != "" {
			return text
		}
	}
	return "Response"
}

var statusDescriptions = map[string]string{
	"200": "Success",
	"201": "Created",
	"202": "Accepted",
	"204": "No Content",
	"400": "Bad Request",
	"401": "Unauthorized",
	"403": "Forbidden",
	"404": "Not Found",
	"409": "Conflict",
	"500": "Internal Error",
	"503": "Service Unavailable",
}

func canonicalContentType(value, fallback string) string {
	if value == "" {
		return fallback
	}
	normalized := strings.ToLower(value)
	switch normalized {
	case "json", "application/json":
		return "application/json"
	case "xml", "application/xml":
		return "application/xml"
	case "yaml", "yml", "application/x-yaml", "application/yaml":
		return "application/x-yaml"
	case "form", "application/x-www-form-urlencoded", "x-www-form-urlencoded":
		return "application/x-www-form-urlencoded"
	case "multipart", "multipart/form-data":
		return "multipart/form-data"
	case "text", "text/plain":
		return "text/plain"
	case "html", "text/html":
		return "text/html"
	default:
		return value
	}
}

// componentBuilder coordinates schema construction for named types.
type componentBuilder struct {
	registry   *TypeRegistry
	components map[string]Schema
	building   map[string]struct{}
}

func newComponentBuilder(reg *TypeRegistry, components map[string]Schema) *componentBuilder {
	return &componentBuilder{
		registry:   reg,
		components: components,
		building:   make(map[string]struct{}),
	}
}

func (b *componentBuilder) ensureComponent(typeName, pkg string) string {
	qual := typeName
	if !strings.Contains(typeName, ".") && pkg != "" {
		qual = pkg + "." + typeName
	}
	compName := buildComponentName(qual)
	if b.components == nil {
		return compName
	}
	if _, exists := b.components[compName]; exists {
		return compName
	}

	var (
		spec *TypeSpecInfo
		key  string
	)
	if b.registry != nil {
		spec, key = b.registry.Resolve(typeName, pkg)
	}
	if key == "" {
		key = qual
	}
	if _, inProgress := b.building[key]; inProgress {
		return compName
	}
	b.building[key] = struct{}{}

	schema := Schema{"type": "object"}
	if spec != nil {
		if built, ok := b.buildSchemaFromSpec(spec); ok && built != nil {
			schema = built
		}
	}

	b.components[compName] = schema
	delete(b.building, key)
	return compName
}

func (b *componentBuilder) buildSchemaFromSpec(info *TypeSpecInfo) (Schema, bool) {
	if info == nil || info.Spec == nil {
		return nil, false
	}

	switch t := info.Spec.Type.(type) {
	case *ast.StructType:
		props := make(map[string]interface{})
		var required []string
		for _, field := range t.Fields.List {
			if len(field.Names) == 0 {
				continue
			}
			for _, name := range field.Names {
				if name == nil || name.Name == "" {
					continue
				}
				meta := extractJSONMetadata(field, name.Name)
				if meta.skip || meta.name == "" {
					continue
				}
				schema := b.schemaFromExpr(field.Type, info.Package)
				if schema == nil {
					continue
				}
				props[meta.name] = schema
				optional := meta.omitEmpty || isPointerType(field.Type)
				if !optional {
					required = append(required, meta.name)
				}
			}
		}
		schema := Schema{"type": "object"}
		if len(props) > 0 {
			schema["properties"] = props
		}
		if len(required) > 0 {
			sort.Strings(required)
			schema["required"] = required
		}
		return schema, true
	case *ast.ArrayType:
		items := b.schemaFromExpr(t.Elt, info.Package)
		if items == nil {
			items = map[string]interface{}{"type": "object"}
		}
		return Schema{
			"type":  "array",
			"items": items,
		}, true
	case *ast.MapType:
		valueSchema := b.schemaFromExpr(t.Value, info.Package)
		if valueSchema == nil {
			valueSchema = map[string]interface{}{"type": "object"}
		}
		return Schema{
			"type":                 "object",
			"additionalProperties": valueSchema,
		}, true
	case *ast.InterfaceType:
		return Schema{"type": "object"}, true
	default:
		if info.Spec.Assign != token.NoPos {
			return b.schemaFromExpr(info.Spec.Type, info.Package), true
		}
		return b.schemaFromExpr(info.Spec.Type, info.Package), true
	}
}

func (b *componentBuilder) schemaFromExpr(expr ast.Expr, pkg string) map[string]interface{} {
	if expr == nil {
		return map[string]interface{}{"type": "object"}
	}
	return schemaOrRef(exprToString(expr), pkg, b)
}

func isPointerType(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.StarExpr:
		return true
	}
	return false
}
