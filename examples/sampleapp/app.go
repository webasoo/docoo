package sampleapp

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Register wires a few sample routes so the doc generator has something to inspect.
func Register(app *fiber.App) {
	app.Get("/health", health)
	app.Get("/users/:id", getUser)
	app.Post("/users", createUser)
}

// @Summary Health check
// @Description Returns basic service status
// @Tags Health
// @Success 200 {object} healthResponse
func health(c *fiber.Ctx) error {
	return c.JSON(healthResponse{Status: "ok"})
}

// @Summary Fetch user
// @Description Returns a user by identifier
// @Tags Users
// @Param id path string true "User identifier"
// @Param verbose query bool false "Include extended profile"
// @Success 200 {object} userResponse
// @Failure 404 {object} errorResponse
func getUser(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusNotFound).JSON(errorResponse{Error: "missing id"})
	}

	name := strings.ReplaceAll(id, "-", " ")
	return c.JSON(userResponse{
		ID:      id,
		Email:   id + "@example.com",
		Name:    strings.ToUpper(name),
		Verbose: c.QueryBool("verbose"),
	})
}

// @Summary Create user
// @Description Parses a JSON body and returns the created record
// @Tags Users
// @Accept json
// @Produce json
// @Param payload body userRequest true "New user"
// @Success 201 {object} userResponse
// @Failure 400 {object} errorResponse
func createUser(c *fiber.Ctx) error {
	var req userRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: "invalid payload"})
	}

	return c.Status(fiber.StatusCreated).JSON(userResponse{
		ID:    "123",
		Email: req.Email,
		Name:  req.Name,
	})
}

// healthResponse is returned by the health endpoint.
type healthResponse struct {
	Status string `json:"status"`
}

// userRequest models the payload accepted by createUser.
type userRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// userResponse models the payload returned by the user endpoints.
type userResponse struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Verbose bool   `json:"verbose,omitempty"`
}

// errorResponse is used for error cases.
type errorResponse struct {
	Error string `json:"error"`
}
