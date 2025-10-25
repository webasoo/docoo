package main

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/webasoo/docoo/core"
	sampleapp "github.com/webasoo/docoo/examples/sampleapp"
	fiberscalar "github.com/webasoo/docoo/fiber-scalar"
	fiberswagger "github.com/webasoo/docoo/fiber-swagger"
)

func main() {
	if _, _, err := core.GenerateAndSaveOpenAPI(); err != nil {
		log.Fatalf("build spec: %v", err)
	}

	app := fiber.New()
	sampleapp.Register(app)
	if err := fiberswagger.Register(app); err != nil {
		log.Fatalf("register swagger: %v", err)
	}
	if err := fiberscalar.Register(app); err != nil {
		log.Fatalf("register scalar: %v", err)
	}

	log.Println("Fiber app running at http://localhost:8080")
	log.Println("Swagger UI available at http://localhost:8080/swagger/")
	log.Println("Scalar UI available at http://localhost:8080/scalar/")

	if err := app.Listen(":8080"); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
