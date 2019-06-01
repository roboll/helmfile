package tmpl

type Context struct {
	preRender bool
	basePath  string
	readFile  func(string) ([]byte, error)
}
