# Route Discovery Notes

These notes describe how `docoo` currently discovers routes and handlers.
They are meant for maintainers (human or AI) who plan to extend the detection
logic or port it to new frameworks.

## High-Level Flow

1. `core.GenerateProjectOpenAPI` resolves the workspace root and directories to
   scan (defaults to the Go module root).
2. `core.FindRoutes` walks each directory, parsing every non-test Go file.
3. An AST pass (`findRoutesInFile`) builds route entries and the supporting
   metadata needed for handler analysis.
4. The discovered `RouteInfo` slice feeds into `BuildHandlerIndex`, which
   extracts annotations, request/response bodies, and schema references.
5. `GenerateOpenAPI` converts the route+handler graphs into the OpenAPI
   document.

## Route Extraction Details (Fiber)

`core/routes.go` is focused on Fiber today.

- We collect import aliases so that `handler.SomeHandler` can be linked to its
  full import path (`alias -> module/path`).
- Group prefixes are tracked by recording the result of `app.Group()` calls in
  both assignment (`foo := app.Group("/api")`) and `var` declarations. Nested
  groups inherit prefixes via `Group` calls.
- Every call expression is inspected; if the selector name is an HTTP verb
  (case-insensitive) and the first argument is a string literal, it is treated
  as a route.
- The path is computed by joining the literal with any stored group prefix.
- Handler resolution handles three cases:
  1. `app.Get("/foo", handlerFn)` ⇒ local ident.
  2. `app.Get("/foo", pkg.Handler)` ⇒ resolved via import alias map.
  3. `svc := external.NewService(); app.Get("/foo", svc.Handler)` ⇒ we trace
     assignments and value specs to map `svc` back to `external` so the import
     path can be determined (`handlerBindings` map).
- When we cannot determine a handler name, the route is skipped (the OpenAPI
  generator needs a stable handler ID to collect docs).

## Handler Analysis Summary

- `core/handlers.go` groups routes by file. Local handlers are parsed from the
  same file; external ones resolve via import path + package directory.
- The parser looks for Swagger-style annotations (`@Summary`, `@Param`, etc.)
  in leading comments. Free-form comments become descriptions.
- Function bodies are inspected to infer request/response types (`BodyParser`,
  `ctx.JSON`, return statements, etc.). The `TypeRegistry` keeps module-wide
  type information to resolve struct definitions.
- Path parameters (`:id` or `*wildcard`) are converted into OpenAPI path params.

## Extending the Scanner

- New frameworks can plug in via additional route-finder implementations that
  populate `RouteInfo`. The rest of the pipeline (handler analysis and OpenAPI
  generation) is shared.
- When adding new handler resolution patterns, update both the route finder and
  `BuildHandlerIndex` if extra metadata is required.
- Keep transformations deterministic; `RouteInfo.HandlerID` (`file::func` or
  `import::func`) is used to correlate routes with handler metadata.

## Known Limitations / TODOs

- Only Fiber routing helpers are recognised today. Chi/Gin support would
  require equivalent AST discovery logic.
- Handler inference is best-effort; complex dependency injection patterns may
  fail to resolve import paths if the object graph is built dynamically.
- Route parameters defined through variables (instead of string literals) are
  currently ignored.

Maintainers can start in `core/routes.go` for route extraction and
`core/handlers.go` for handler semantics. Update this document whenever the
discovery rules change so future automation stays in sync.
