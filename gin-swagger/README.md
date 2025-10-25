# gin-swagger

Adapter that exposes the embedded Swagger UI on a Gin router.

## Installation

```bash
go get github.com/webasoo/docoo/gin-swagger
```

Make sure `openapi.json` exists (e.g. run `docoo generate`) before mounting
the routes.

## Usage

```go
router := gin.Default()
if err := ginswagger.RegisterFile(router, "openapi.json"); err != nil {
    log.Fatal(err)
}
router.Run(":8080")
```

`Handler(spec []byte)` and `Register(router, spec)` are available if you already
hold the JSON in memory.

## License

Distributed under the [DOCOO Community License v1.0](../LICENSE).
