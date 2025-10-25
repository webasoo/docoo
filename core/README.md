# core

The `core` package discovers HTTP routes in Go codebases, extracts handler
metadata, and emits an OpenAPI 3 document. It is framework-agnostic: adapters
simply consume the generated JSON.

## Installation

```bash
go get github.com/webasoo/docoo/core
```

In most projects the easiest way to produce `openapi.json` is to run the CLI that
ships with this module:

```bash
docoo generate
```

If you need to embed the generator directly, you can still use the library
primitives.

## Generate a Spec in Code

```go
routes, err := core.FindRoutes("./internal/server")
if err != nil {
    log.Fatalf("discover routes: %v", err)
}
handlers, registry, err := core.BuildHandlerIndex(routes, ".")
if err != nil {
    log.Fatalf("analyse handlers: %v", err)
}
spec, err := core.GenerateOpenAPI(routes, handlers, registry)
if err != nil {
    log.Fatalf("generate openapi: %v", err)
}
if err := os.WriteFile("openapi.json", spec, 0o644); err != nil {
    log.Fatalf("write file: %v", err)
}
```

## CLI Convenience

```bash
go run github.com/webasoo/docoo generate -o api/openapi.json
```

Add it to `go generate` if you prefer automation:

```go
//go:generate go run github.com/webasoo/docoo generate
package yourpackage
```

## License

Released under the [DOCOO Community License v1.0](../LICENSE). Non-commercial
use is allowed; contact the author for commercial terms.
