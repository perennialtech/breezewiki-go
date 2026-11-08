package breezewiki

import (
	"io/fs"
	"testing"
)

func TestEmbeddedAssets(t *testing.T) {
	for _, tc := range []struct {
		fs   fs.FS
		name string
	}{
		{StaticFiles(), "main.css"},
		{StaticFiles(), "jsonp.js"},
		{SolrConfigFiles(), "schema.xml"},
		{SolrConfigFiles(), "lang/stopwords_en.txt"},
	} {
		if _, err := fs.Stat(tc.fs, tc.name); err != nil {
			t.Errorf("embedded asset %q: %v", tc.name, err)
		}
	}
}
