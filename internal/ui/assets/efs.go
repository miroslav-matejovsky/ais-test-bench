package assets

import (
	"embed"
	"io/fs"
)

//go:embed "html" "static"
var files embed.FS

var (
	// HTMLFiles contains the html/template files.
	HTMLFiles = sub(files, "html")
	// StaticFiles contains files served as-is under /static/.
	StaticFiles = sub(files, "static")
)

// sub panics on error. fs.Sub fails only for invalid paths, and the paths
// passed here are constant and valid.
func sub(f embed.FS, dir string) fs.FS {
	s, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return s
}
