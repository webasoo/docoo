package streaming

func Register(app *App) {
	app.Get("/dynamic/alpha", dynamicHandler)
	app.Get("/dynamic/beta", dynamicHandler)
	app.Get("/download/:name", downloadHandler)
	app.Get("/stream", streamHandler)
	app.Head("/ping", pingHandler)
}

func dynamicHandler(c *Ctx) error {
	return c.Status(200).JSON(map[string]string{
		"message": "ok",
	})
}

func downloadHandler(c *Ctx) error {
	return c.SendFile("./files/report.pdf")
}

func streamHandler(c *Ctx) error {
	reader := &Stream{}
	return c.Status(200).SendStream(reader)
}

func pingHandler(c *Ctx) error {
	return c.SendStatus(204)
}
