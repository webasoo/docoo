package multipart

type App struct{}

func (a *App) Post(path string, handler interface{}) {}

type Ctx struct{}

func (c *Ctx) FormFile(name string) (*FileHeader, error) { return nil, nil }
func (c *Ctx) FormValue(name string, defaultValue ...string) string {
	return ""
}
func (c *Ctx) MultipartForm() (*Form, error) { return &Form{}, nil }
func (c *Ctx) SaveFile(file *FileHeader, dst string) error {
	return nil
}
func (c *Ctx) Status(code int) *Ctx         { return c }
func (c *Ctx) JSON(value interface{}) error { return nil }

type FileHeader struct {
	Filename string
}

type Form struct {
	File map[string][]*FileHeader
}
