package textresponse

func Register(app *App) {
	app.Get("/hello", helloHandler)
	app.Get("/accepted", acceptedHandler)
	app.Get("/go", redirectHandler)
}

func helloHandler(c *Ctx) error {
	return c.SendString("hello world")
}

func acceptedHandler(c *Ctx) error {
	return c.SendStatus(202)
}

func redirectHandler(c *Ctx) error {
	return c.Redirect("/target", 302)
}
