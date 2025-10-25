package streaming

type App struct{}

func (a *App) Get(path string, handler interface{})  {}
func (a *App) Head(path string, handler interface{}) {}

type Ctx struct{}

func (c *Ctx) Status(code int) *Ctx         { return c }
func (c *Ctx) JSON(value interface{}) error { return nil }
func (c *Ctx) SendStatus(code int) error    { return nil }
func (c *Ctx) SendFile(path string) error   { return nil }
func (c *Ctx) SendStream(stream interface{}) error {
	return nil
}

type Stream struct{}

func (s *Stream) Read(p []byte) (int, error) { return 0, nil }
