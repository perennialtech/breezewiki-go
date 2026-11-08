// Package breezewiki includes the static assets and offline indexer resources
// in Go binaries, so installed programs do not need the source checkout.
package breezewiki

import (
	"embed"
	"io/fs"
)

//go:embed static archiver/solr-config-dir
var files embed.FS

// StaticFiles contains the CSS, JavaScript, fonts and artwork used by the site.
func StaticFiles() fs.FS {
	sub, _ := fs.Sub(files, "static")
	return sub
}

// SolrConfigFiles contains the upstream search schema and language resources.
func SolrConfigFiles() fs.FS {
	sub, _ := fs.Sub(files, "archiver/solr-config-dir")
	return sub
}
