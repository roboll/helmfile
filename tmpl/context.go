package tmpl

type Context struct {
	readFile func(string) ([]byte, error)
}
