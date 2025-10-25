package textresponse

type App struct{}

func (a *App) Get(path string, handler interface{}) {}

type Ctx struct{}

func (c *Ctx) Status(code int) *Ctx          { return c }
func (c *Ctx) JSON(value interface{}) error  { return nil }
func (c *Ctx) SendStatus(code int) error     { return nil }
func (c *Ctx) SendString(value string) error { return nil }
func (c *Ctx) Redirect(location string, status ...int) error {
	return nil
}
