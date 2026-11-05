package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web
var webFS embed.FS

// registerUI serves the portal. It is plain HTML, CSS and JS with no build
// step; the page talks to the JSON API with the user's session cookie.
func registerUI(mux *http.ServeMux) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err) // the embedded directory is fixed at build time
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, sub, "index.html")
	})
	mux.Handle("GET /ui/", http.StripPrefix("/ui/", http.FileServerFS(sub)))
}
