package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGenerateProjectOpenAPI_Fixtures(t *testing.T) {
	fixtures := []string{
		"mixed",
		"queryvariants",
		"multipart",
		"streaming",
		"textresponse",
	}

	for _, name := range fixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixtureRoot := filepath.Join("testdata", "projects", name)
			spec, err := GenerateProjectOpenAPI(ProjectConfig{WorkspaceRoot: fixtureRoot})
			if err != nil {
				routes, routeErr := FindRoutes(fixtureRoot)
				t.Fatalf("GenerateProjectOpenAPI(%s) error = %v (routes=%d, first=%v, routeErr=%v)", name, err, len(routes), firstRouteDebug(routes), routeErr)
			}

			actual := normalizeJSON(t, spec)

			goldenPath := filepath.Join(fixtureRoot, "expected_openapi.json")
			if update := os.Getenv("DOCLESS_UPDATE_GOLDEN"); update != "" {
				if err := os.WriteFile(goldenPath, spec, 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
			}

			goldenBytes, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			expected := normalizeJSON(t, goldenBytes)

			if !reflect.DeepEqual(expected, actual) {
				t.Fatalf("OpenAPI mismatch.\nExpected: %s\nActual:   %s", mustMarshal(t, expected), mustMarshal(t, actual))
			}
		})
	}
}

func normalizeJSON(t *testing.T, data []byte) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, string(data))
	}
	return v
}

func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

func firstRouteDebug(routes []RouteInfo) interface{} {
	if len(routes) == 0 {
		return nil
	}
	r := routes[0]
	return map[string]interface{}{
		"file":      r.File,
		"handlerID": r.HandlerID,
		"handler":   r.HandlerName,
	}
}
