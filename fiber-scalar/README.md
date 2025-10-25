# Fiber Scalar

`fiber-scalar` mounts the embedded Scalar API Reference UI from the `scalar`
package onto a Fiber application.

```go
package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
	fiberscalar "github.com/webasoo/docoo/fiber-scalar"
)

func main() {
	app := fiber.New()

	// register your API routes first …

	if err := fiberscalar.RegisterFile(app, "openapi.json"); err != nil {
		log.Fatal(err)
	}

	app.Listen(":8080")
}
```

You can skip `RegisterFile` and call `Register` if your spec lives at the
default `<module root>/openapi.json`.

To avoid reloading the document from disk you can generate it once and use
`RegisterWithSpec(app, specBytes)`.
