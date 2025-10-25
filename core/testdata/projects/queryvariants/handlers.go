package queryvariants

func Register(app *App) {
	app.Get("/search", searchHandler)
}

func searchHandler(c *Ctx) error {
	page := c.QueryInt("page", 1)
	if page <= 0 {
		return c.Status(400).JSON(map[string]string{"error": "invalid page"})
	}

	includeArchived := c.QueryBool("archived")
	resultLimit := int(c.QueryFloat("limit"))

	var filters Filter
	if err := c.QueryParser(&filters); err != nil {
		return c.Status(400).JSON(map[string]string{"error": "invalid filters"})
	}

	args := struct {
		Page     int    `json:"page"`
		Archived bool   `json:"archived"`
		Limit    int    `json:"limit"`
		Tag      string `json:"tag"`
	}{
		Page:     page,
		Archived: includeArchived,
		Limit:    resultLimit,
		Tag:      filters.Tag,
	}

	return c.Status(200).JSON(args)
}
