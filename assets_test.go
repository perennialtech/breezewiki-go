package breezewiki

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedAssets(t *testing.T) {
	for _, tc := range []struct {
		fs   fs.FS
		name string
	}{
		{StaticFiles(), "main.css"},
		{StaticFiles(), "jsonp.js"},
	} {
		if _, err := fs.Stat(tc.fs, tc.name); err != nil {
			t.Errorf("embedded asset %q: %v", tc.name, err)
		}
	}
}

func TestJSONPPostFailureHandling(t *testing.T) {
	asset, err := fs.ReadFile(StaticFiles(), "jsonp.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(asset)
	postError := strings.Index(js, `if (xhr.status !== 200) return error(`)
	responseXML := strings.Index(js, `xhr.responseXML`)
	if postError < 0 || responseXML < postError {
		t.Error("JSONP does not handle non-500 HTTP failures before using responseXML")
	}
	if !strings.Contains(js, `xhr.addEventListener("error"`) {
		t.Error("JSONP does not handle POST network failures")
	}
}
