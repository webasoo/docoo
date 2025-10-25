package mixed

type App struct{}

func (a *App) Get(path string, handler interface{}) {}
func (a *App) Post(path string, handler interface{}) {}
func (a *App) Group(prefix string) *App { return a }

type Ctx struct{}

func (c *Ctx) Query(key string, defaultValue ...string) string { return "" }
func (c *Ctx) FormFile(name string) (*FileHeader, error)       { return nil, nil }
func (c *Ctx) BodyParser(v interface{}) error                  { return nil }
func (c *Ctx) JSON(value interface{}) error                    { return nil }
func (c *Ctx) Status(code int) *Ctx                            { return c }

type FileHeader struct{}

func (fh *FileHeader) Open() (File, error) { return nil, nil }

type File interface {
	Read(p []byte) (n int, err error)
	Close() error
}

func OKResult(c *Ctx, payload interface{}) error { return nil }
func BadRequest(c *Ctx, msg string) error        { return nil }
func NotFound(c *Ctx, msg string) error          { return nil }
func InternalError(c *Ctx, msg string) error     { return nil }
