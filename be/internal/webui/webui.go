// Package webui serves the dashboard single-page app that is embedded into the
// binary at build time.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:static
var static embed.FS

const prefix = "/ui/"

// Handler serves the embedded build under /ui/. Until a build has put an
// index.html into static/, every path answers 503.
func Handler() http.Handler {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		// Unreachable: the embed directive guarantees the directory exists.
		sub = static
	}
	return handler(sub)
}

// handler is Handler over any file system, so tests can cover both states.
func handler(fsys fs.FS) http.Handler {
	files := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(fsys, "index.html"); err != nil {
			http.Error(w, "UI chưa được build", http.StatusServiceUnavailable)
			return
		}
		// Cleaning a rooted path cannot climb out of fsys.
		name := path.Clean("/" + strings.TrimPrefix(r.URL.Path, prefix))[1:]
		if path.Ext(name) == "" {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFileFS(w, r, fsys, "index.html")
			return
		}
		if info, err := fs.Stat(fsys, name); err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		r2 := r.Clone(r.Context())
		u := *r.URL
		u.Path, u.RawPath = "/"+name, ""
		r2.URL = &u
		files.ServeHTTP(w, r2)
	})
}
