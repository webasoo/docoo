# docoo

`docoo` is a modular toolkit for turning Go HTTP handlers into OpenAPI 3
specifications and shipping the docs with zero framework lock-in. Route sampling,
handler analysis, static UIs, and framework adapters are provided as separate
packages so you only import what you need.

- **Language:** Go 1.22+
- **CLI:** `docoo` (installed via `go install`; the binary adapts to whatever name you invoke it with)
- **License:** [DOCOO Community License v1.0](./LICENSE)

## Packages & Use Cases

| Package              | Import Path                              | Purpose                                                         |
| -------------------- | ---------------------------------------- | --------------------------------------------------------------- |
| Core engine          | `github.com/webasoo/docoo/core`          | Route discovery, handler metadata, OpenAPI JSON generation      |
| Swagger UI           | `github.com/webasoo/docoo/swagger`       | Embedded Swagger UI assets served via `net/http`                |
| Redoc UI             | `github.com/webasoo/docoo/redoc`         | Embedded Redoc viewer served via `net/http`                     |
| Scalar UI            | `github.com/webasoo/docoo/scalar`        | Embedded Scalar API Reference served via `net/http`             |
| Fiber adapter        | `github.com/webasoo/docoo/fiber-swagger` | Wraps Swagger handler for Fiber apps; auto-loads `openapi.json` |
| Fiber Scalar adapter | `github.com/webasoo/docoo/fiber-scalar`  | Mounts Scalar UI on Fiber apps                                  |
| Chi adapter          | `github.com/webasoo/docoo/chi-swagger`   | Mounts Swagger UI on Chi routers                                |
| Gin adapter          | `github.com/webasoo/docoo/gin-swagger`   | Mounts Swagger UI on Gin routers                                |

Install only what you require; Go will fetch shared dependencies automatically.

## Installation

```bash
# Install the CLI (installs the `docoo` binary)
go install github.com/webasoo/docoo@latest

# Add any adapter or helper package to your project
go get github.com/webasoo/docoo/fiber-swagger
```

## Generating Documentation

The CLI exposes a single sub-command:

```bash
# generate <module-root>/openapi.json
cd /path/to/your/project
recipient@host$ docoo generate
```

Key flags:

```bash
-o <path>        # write to a custom file (relative paths resolved from module root)
-root <path>     # project/module root to scan (defaults to cwd module)
-route <dir>     # add extra directories to scan for routes (repeatable)
-skip <prefix>   # ignore URLs with the given prefix (repeatable)
```

For automation you can wire the CLI into Go’s generation workflow:

```go
//go:generate go run github.com/webasoo/docoo generate
package yourpackage
```

Running `go generate ./...` will refresh `openapi.json`.

## Serving the UI

Once the spec exists, add an adapter that fits your stack.

Import whichever viewers you need, for example `github.com/webasoo/docoo/swagger`,
`github.com/webasoo/docoo/redoc`, or `github.com/webasoo/docoo/scalar`.

### Standard Library

```go
if err := swagger.RegisterFile("openapi.json"); err != nil {
    log.Fatal(err)
}
if err := redoc.RegisterFile("openapi.json"); err != nil {
    log.Fatal(err)
}
if err := scalar.RegisterFile("openapi.json"); err != nil {
    log.Fatal(err)
}
log.Fatal(http.ListenAndServe(":8080", nil))
```

### Fiber

```go
app := fiber.New()
// register your handlers first …
if err := fiberswagger.Register(app); err != nil {
    log.Fatal(err)
}
if err := fiberscalar.Register(app); err != nil {
    log.Fatal(err)
}
app.Listen(":8080")
```

### Chi / Gin

```go
if err := chiswagger.RegisterFile(router, "openapi.json"); err != nil {
    log.Fatal(err)
}
if err := ginswagger.RegisterFile(router, "openapi.json"); err != nil {
    log.Fatal(err)
}
```

## Examples

The `examples/` directory contains runnable samples that demonstrate:

- `examples/http` – standard library server that serves Swagger + Redoc + Scalar
- `examples/fiber` – Fiber server with Swagger + Scalar adapters

Run them after generating docs:

```bash
GOCACHE=$(pwd)/.gocache go run ./examples/http
```

## Development Notes

- Go 1.22 or newer is required.
- Static assets are vendored via `go:generate go generate ./swagger ./redoc ./scalar`.
- Tests (TODO) will cover route discovery, CLI, and adapters.
- See `docs/ROUTE_DISCOVERY.md` for an overview of how route/handler detection works.

## License

This project is distributed under the [DOCOO Community License v1.0](./LICENSE).
Non-commercial usage is permitted. Commercial use or public forks require written
approval from the author.

For commercial licensing enquiries, email [amin@webasoo.com](mailto:amin@webasoo.com).
