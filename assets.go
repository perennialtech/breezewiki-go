// Package breezewiki includes the static assets in Go binaries, so installed
// programs do not need the source checkout.
package breezewiki

import (
	"embed"
	"io/fs"
)

//go:embed static
var files embed.FS

// StaticFiles contains the CSS, JavaScript, fonts and artwork used by the site.
func StaticFiles() fs.FS {
	sub, _ := fs.Sub(files, "static")
	return sub
}
