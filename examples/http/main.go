package main

import (
	"log"
	"net/http"

	"github.com/webasoo/docoo/core"
	"github.com/webasoo/docoo/redoc"
	"github.com/webasoo/docoo/scalar"
	"github.com/webasoo/docoo/swagger"
)

func main() {
	path, _, err := core.GenerateAndSaveOpenAPI()
	if err != nil {
		log.Fatalf("build spec: %v", err)
	}

	if err := swagger.RegisterFile(path); err != nil {
		log.Fatalf("register swagger: %v", err)
	}
	if err := redoc.RegisterFile(path); err != nil {
		log.Fatalf("register redoc: %v", err)
	}

	if err := scalar.RegisterFile(path); err != nil {
		log.Fatalf("register scalar spec: %v", err)
	}

	log.Println("Scalar UI available at http://localhost:8080/scalar/")
	log.Println("Swagger UI available at http://localhost:8080/swagger/")
	log.Println("Redoc UI available at http://localhost:8080/redoc/")

	log.Fatal(http.ListenAndServe(":8080", nil))
}
