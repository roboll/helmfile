package tmpl

type Context struct {
	basePath string
	readFile func(string) ([]byte, error)
}
