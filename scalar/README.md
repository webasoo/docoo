# Scalar UI

The `scalar` package embeds the [Scalar API Reference](https://github.com/scalar/scalar) UI
so you can serve interactive OpenAPI docs straight from your Go binary.

```go
package main

import (
	"log"
	"net/http"

	"github.com/webasoo/docoo/scalar"
)

func main() {
	if err := scalar.RegisterFile("openapi.json"); err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

## Updating Assets

Assets are pulled from jsDelivr and vendored into this repository so binaries
remain self-contained.

```bash
SCALAR_VERSION=1.38.1 go generate ./scalar
```

By default the `tools/build.sh` script fetches version `1.38.1`; override the
`SCALAR_VERSION` environment variable to select a different release.
