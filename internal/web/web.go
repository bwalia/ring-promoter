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
	"path"
	"strings"
)

// The all: prefix is required: the Next.js export contains _next/, and plain
// go:embed patterns skip files and directories starting with "_" or ".".
//
//go:embed all:static
var files embed.FS

// Handler returns an http.Handler serving the embedded static assets from the
// root path.
//
// The Next.js export writes non-root routes as flat files ("landing.html",
// not "landing/index.html"), which http.FileServer alone would not resolve
// for a clean URL like /landing — so extensionless paths fall back to the
// matching .html file when one exists.
func Handler() http.Handler {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" && p != "." && path.Ext(p) == "" {
			if f, err := sub.Open(p + ".html"); err == nil {
				f.Close()
				http.ServeFileFS(w, r, sub, p+".html")
				return
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}
