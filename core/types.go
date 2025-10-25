package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// TypeRegistry tracks type specifications discovered while scanning source files.
type TypeRegistry struct {
	packages         map[string]map[string]*TypeSpecInfo
	functions        map[string][]FuncSignature
	indexedWorkspace bool
}

// FuncSignature captures a function's return types within a package.
type FuncSignature struct {
	Package string
	Results []string
}

// TypeSpecInfo stores metadata about a type declaration.
type TypeSpecInfo struct {
	Package string
	Name    string
	File    string
	Spec    *ast.TypeSpec
}

// NewTypeRegistry constructs an empty registry.
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{
		packages:  make(map[string]map[string]*TypeSpecInfo),
		functions: make(map[string][]FuncSignature),
	}
}

// Add records a type specification for later schema generation.
func (r *TypeRegistry) Add(pkg, file string, spec *ast.TypeSpec) {
	if r == nil || spec == nil || spec.Name == nil || spec.Name.Name == "" {
		return
	}
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		pkg = "main"
	}
	pkgMap := r.packages[pkg]
	if pkgMap == nil {
		pkgMap = make(map[string]*TypeSpecInfo)
		r.packages[pkg] = pkgMap
	}
	name := spec.Name.Name
	if _, exists := pkgMap[name]; exists {
		return
	}
	pkgMap[name] = &TypeSpecInfo{Package: pkg, Name: name, File: file, Spec: spec}
}

// AddFunction registers a function or method signature.
func (r *TypeRegistry) AddFunction(pkg, name string, results []string) {
	if r == nil || name == "" || len(results) == 0 {
		return
	}
	sig := FuncSignature{Package: pkg, Results: append([]string(nil), results...)}
	existing := r.functions[name]
	for _, cur := range existing {
		if cur.Package == sig.Package && equalStringSlices(cur.Results, sig.Results) {
			return
		}
	}
	r.functions[name] = append(existing, sig)
}

// LookupFunction returns result types for a function name when the signature is unambiguous.
func (r *TypeRegistry) LookupFunction(name string, expected int) ([]string, bool) {
	if r == nil || name == "" {
		return nil, false
	}
	candidates := r.functions[name]
	if len(candidates) == 0 {
		return nil, false
	}
	var match []string
	for _, sig := range candidates {
		if expected > 0 && len(sig.Results) != expected {
			continue
		}
		if match != nil {
			return nil, false
		}
		match = sig.Results
	}
	if match == nil {
		if expected == 0 && len(candidates) == 1 {
			return candidates[0].Results, true
		}
		return nil, false
	}
	return match, true
}

// Resolve locates a type declaration by name, using the default package when needed.
func (r *TypeRegistry) Resolve(typeName, defaultPkg string) (*TypeSpecInfo, string) {
	if r == nil || typeName == "" {
		return nil, ""
	}
	pkg := defaultPkg
	name := typeName
	if idx := strings.Index(typeName, "."); idx > 0 {
		pkg = typeName[:idx]
		name = typeName[idx+1:]
	}
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return nil, ""
	}
	if pkgMap := r.packages[pkg]; pkgMap != nil {
		if info := pkgMap[name]; info != nil {
			return info, pkg + "." + name
		}
	}
	var (
		candidates    []*TypeSpecInfo
		candidatePkgs []string
	)
	for pkgName, pkgMap := range r.packages {
		if info := pkgMap[name]; info != nil {
			candidates = append(candidates, info)
			candidatePkgs = append(candidatePkgs, pkgName)
		}
	}
	switch len(candidates) {
	case 0:
		return nil, pkg + "." + name
	case 1:
		return candidates[0], candidatePkgs[0] + "." + name
	}
	aliasLower := strings.ToLower(pkg)
	var (
		best    *TypeSpecInfo
		bestPkg string
	)
	for i, info := range candidates {
		pkgLower := strings.ToLower(candidatePkgs[i])
		if aliasLower == "" {
			continue
		}
		if aliasLower == pkgLower || strings.HasSuffix(aliasLower, pkgLower) {
			if best != nil {
				return nil, pkg + "." + name
			}
			best = info
			bestPkg = candidatePkgs[i]
		}
	}
	if best != nil {
		return best, bestPkg + "." + name
	}
	defaultLower := strings.ToLower(strings.TrimSpace(defaultPkg))
	if defaultLower != "" {
		for i, info := range candidates {
			if strings.ToLower(candidatePkgs[i]) == defaultLower {
				return info, candidatePkgs[i] + "." + name
			}
		}
	}
	return nil, pkg + "." + name
}

// IndexWorkspace walks the module rooted at dir and records type and function information.
func (r *TypeRegistry) IndexWorkspace(root string) error {
	if r == nil || root == "" || r.indexedWorkspace {
		return nil
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		node, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return parseErr
		}
		pkgName := node.Name.Name
		for _, decl := range node.Decls {
			switch typed := decl.(type) {
			case *ast.GenDecl:
				if typed.Tok != token.TYPE {
					continue
				}
				for _, spec := range typed.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						r.Add(pkgName, path, ts)
					}
				}
			case *ast.FuncDecl:
				if typed.Type == nil || typed.Type.Results == nil {
					continue
				}
				results := collectResultTypes(typed.Type.Results)
				if len(results) == 0 {
					continue
				}
				r.AddFunction(pkgName, typed.Name.Name, results)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.indexedWorkspace = true
	return nil
}

func collectResultTypes(list *ast.FieldList) []string {
	if list == nil {
		return nil
	}
	var results []string
	for _, field := range list.List {
		typeStr := exprToString(field.Type)
		if typeStr == "" {
			typeStr = "interface{}"
		}
		if len(field.Names) == 0 {
			results = append(results, typeStr)
			continue
		}
		for range field.Names {
			results = append(results, typeStr)
		}
	}
	return results
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// jsonFieldMetadata describes details extracted from struct field tags.
type jsonFieldMetadata struct {
	name      string
	omitEmpty bool
	skip      bool
}

func extractJSONMetadata(field *ast.Field, fallback string) jsonFieldMetadata {
	if field == nil {
		return jsonFieldMetadata{name: fallback}
	}
	rawTag := ""
	if field.Tag != nil {
		if unquoted, err := strconv.Unquote(field.Tag.Value); err == nil {
			rawTag = unquoted
		}
	}
	if rawTag == "" {
		return jsonFieldMetadata{name: fallback}
	}
	tag := reflect.StructTag(rawTag)
	jsonTag := tag.Get("json")
	if jsonTag == "" {
		return jsonFieldMetadata{name: fallback}
	}
	if jsonTag == "-" {
		return jsonFieldMetadata{skip: true}
	}
	parts := strings.Split(jsonTag, ",")
	name := parts[0]
	if name == "" {
		name = fallback
	}
	meta := jsonFieldMetadata{name: name}
	for _, opt := range parts[1:] {
		if strings.TrimSpace(opt) == "omitempty" {
			meta.omitEmpty = true
		}
	}
	return meta
}
