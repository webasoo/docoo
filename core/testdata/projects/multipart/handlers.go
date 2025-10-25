package multipart

func Register(app *App) {
	app.Post("/submit", submitHandler)
}

func submitHandler(c *Ctx) error {
	name := c.FormValue("name")
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(400).JSON(map[string]string{"error": "invalid form"})
	}
	files := form.File["attachments"]
	for _, file := range files {
		_ = c.SaveFile(file, "./uploads/"+file.Filename)
	}
	response := struct {
		Name        string `json:"name"`
		Attachments int    `json:"attachments"`
	}{
		Name:        name,
		Attachments: len(files),
	}
	return c.Status(201).JSON(response)
}
