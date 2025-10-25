# fiber-swagger

Adapter that mounts the embedded Swagger UI on a Fiber application. It reads
`openapi.json` from the module root by default.

## Installation

```bash
go get github.com/webasoo/docoo/fiber-swagger
```

Run `docoo generate` (or the `docoo` CLI) first so `openapi.json` exists
at the module root.

## Usage

```go
app := fiber.New()
// Register your API routes…

if err := fiberswagger.Register(app); err != nil {
    log.Fatal(err)
}

app.Listen(":8080")
```

### Custom Spec Location

```go
if err := fiberswagger.RegisterFile(app, "api/openapi.json"); err != nil {
    log.Fatal(err)
}
```

## License

Distributed under the [DOCOO Community License v1.0](../LICENSE).
