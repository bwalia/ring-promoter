// Package web embeds and serves the single-page UI from the binary.
//
// The UI is a Next.js app in web/ at the repository root, exported to static
// files. After changing it, run `npm run build:embed` in web/ to regenerate
// the contents of the static/ directory embedded here.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// The all: prefix is required: the Next.js export contains _next/, and plain
// go:embed patterns skip files and directories starting with "_" or ".".
//
//go:embed all:static
var files embed.FS

// Handler returns an http.Handler serving the embedded static assets from the
// root path.
func Handler() http.Handler {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
