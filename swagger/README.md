# swagger

Serves the embedded Swagger UI distribution through `net/http`. Provide a byte
slice or point it at a `openapi.json` file.

## Installation

```bash
go get github.com/webasoo/docoo/swagger
```

Generate `openapi.json` first (for example with `docoo generate`) before
mounting the handler.

## Quick Usage

```go
// Load the spec from disk
handler, err := swagger.HandlerFromFile("openapi.json")
if err != nil {
    log.Fatal(err)
}
http.Handle("/swagger", handler)
http.Handle("/swagger/", handler)
log.Fatal(http.ListenAndServe(":8080", nil))
```

### Inline Spec

```go
spec := []byte(`{"openapi":"3.0.0"}`)
http.Handle("/swagger/", swagger.Handler(spec))
```

## Updating Assets

```
SWAGGER_UI_VERSION=5.11.0 go generate ./swagger
```

## License

Distributed under the [DOCOO Community License v1.0](../LICENSE).
