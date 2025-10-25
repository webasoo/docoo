package queryvariants

type App struct{}

func (a *App) Get(path string, handler interface{}) {}
func (a *App) Post(path string, handler interface{}) {}
func (a *App) Group(prefix string) *App { return a }

type Ctx struct{}

func (c *Ctx) Query(key string, defaultValue ...string) string { return "" }
func (c *Ctx) QueryInt(key string, defaultValue ...int) int    { return 0 }
func (c *Ctx) QueryBool(key string, defaultValue ...bool) bool { return false }
func (c *Ctx) QueryFloat(key string, defaultValue ...float64) float64 {
	return 0
}
func (c *Ctx) QueryParser(out interface{}) error { return nil }
func (c *Ctx) BodyParser(v interface{}) error    { return nil }
func (c *Ctx) JSON(value interface{}) error      { return nil }
func (c *Ctx) Status(code int) *Ctx              { return c }
func (c *Ctx) SendStatus(code int) error         { return nil }

type Filter struct {
	Tag   string `query:"tag"`
	Limit int    `query:"limit"`
}
