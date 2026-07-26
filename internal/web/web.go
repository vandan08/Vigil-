// Package web embeds the incident console. The dashboard ships inside the
// binary — no Node, no build step, no CDN — so `go build` remains the whole
// toolchain and the single-binary deploy story holds (ADR-004).
package web

import (
	"embed"
	"net/http"
)

//go:embed static
var assets embed.FS

// Index serves the console shell at the site root.
func Index(w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, assets, "static/index.html")
}

// Static serves the console's assets. Embedded paths already carry the
// static/ prefix, so the handler mounts at /static/ without stripping.
func Static() http.Handler {
	return http.FileServerFS(assets)
}
