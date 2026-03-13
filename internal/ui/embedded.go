package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var dist embed.FS

// FS returns a filesystem rooted at dist/.
func FS() fs.FS {
	fsys, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return fsys
}

// Handler returns an HTTP handler that serves embedded static files.
func Handler() http.Handler {
	return http.FileServer(http.FS(FS()))
}

// IndexHTML returns the content of dist/index.html, or nil if not built yet.
func IndexHTML() []byte {
	b, _ := dist.ReadFile("dist/index.html")
	return b
}
