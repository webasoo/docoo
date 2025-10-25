package mixed

import "strings"

func Register(app *App) {
	app.Get("/status", healthHandler)

	series := app.Group("/series")
	series.Get("/history/:sourceId", historyBySourceHandler)
	series.Get("/cheapest", cheapestHandler)

	admin := app.Group("/admin")
	admin.Post("/upload", uploadHandler)

	app.Post("/compute", computeHandler)
}

func healthHandler(c *Ctx) error {
	return c.Status(200).JSON(map[string]interface{}{
		"status": "ok",
	})
}

func historyBySourceHandler(c *Ctx) error {
	sourceID := strings.TrimSpace(c.Query("sourceId"))
	if sourceID == "" {
		return BadRequest(c, "sourceId required")
	}
	limits := []string{}
	limitValue := c.Query("limit")
	if limitValue != "" {
		limits = append(limits, limitValue)
	}
	return OKResult(c, HistoryResponse{
		SourceID: sourceID,
		Limits:   limits,
		Items: []HistoryItem{
			{ID: "1", Change: "created"},
		},
	})
}

func cheapestHandler(c *Ctx) error {
	titles := []string{}
	titleValue := c.Query("title")
	if titleValue != "" {
		titles = append(titles, titleValue)
	}
	result := struct {
		Titles []string `json:"titles"`
	}{
		Titles: titles,
	}
	return OKResult(c, result)
}

func uploadHandler(c *Ctx) error {
	if _, err := c.FormFile("file"); err != nil {
		return BadRequest(c, "file required")
	}
	return OKResult(c, UploadResponse{Processed: 1})
}

func computeHandler(c *Ctx) error {
	var payload struct {
		Label  string   `json:"label"`
		Values []int    `json:"values"`
		Tags   []string `json:"tags,omitempty"`
	}
	if err := c.BodyParser(&payload); err != nil || payload.Label == "" {
		return BadRequest(c, "invalid payload")
	}
	resp := ComputeResponse{
		ID:    payload.Label,
		Count: len(payload.Values),
	}
	return c.Status(201).JSON(resp)
}

type HistoryResponse struct {
	SourceID string        `json:"sourceId"`
	Limits   []string      `json:"limits,omitempty"`
	Items    []HistoryItem `json:"items"`
}

type HistoryItem struct {
	ID     string `json:"id"`
	Change string `json:"change"`
}

type UploadResponse struct {
	Processed int `json:"processed"`
}

type ComputeResponse struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}
