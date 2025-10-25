# chi-swagger

Expose the embedded Swagger UI on a Chi router.

## Installation

```bash
go get github.com/webasoo/docoo/chi-swagger
```

Create `openapi.json` beforehand (for example via `docoo generate`).

## Usage

```go
router := chi.NewRouter()
if err := chiswagger.RegisterFile(router, "openapi.json"); err != nil {
    log.Fatal(err)
}
http.ListenAndServe(":8080", router)
```

Need in-memory control? Use `chiswagger.Handler(spec []byte)` and wire it to any
pattern yourself.

## License

Distributed under the [DOCOO Community License v1.0](../LICENSE).
