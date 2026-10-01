package web

import (
	"embed"
	"io/fs"
)

// assets contains the browser application so the server can run as one binary.
//
//go:embed index.html app.js styles.css
var assets embed.FS

// FS returns the embedded frontend filesystem.
func FS() fs.FS {
	return assets
}
