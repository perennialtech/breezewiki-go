package archive

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLegacyCacheRoundTrip(t *testing.T) {
	docs := []Document{
		{ID: "17", Title: "Quoted \"name\"/Título", Body: "A line\nAnother line\\ with text\t\x00", Table: "<html>&", Len: 36},
		{ID: "200", Title: "Next", Body: "", Table: "", Len: 0},
	}
	path := filepath.Join(t.TempDir(), "cache.rkt")
	if err := writeIndexCache(path, docs); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "#hasheq((id . ") {
		t.Fatalf("cache does not use Racket's legacy syntax: %s", contents)
	}
	read, err := readIndexCache(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(read, docs) {
		t.Errorf("Racket cache round trip = %+v, want %+v", read, docs)
	}
}

func TestReadExistingRacketCache(t *testing.T) {
	// The original Racket writer is free to order hash fields differently.
	body := `(#hasheq((body . "Hello\nWorld") (len . 11) (table . "cell") (title . "Téo") (id . "77")) #hasheq((table . "") (title . "Hi") (id . "78") (len . 0) (body . "")))`
	path := filepath.Join(t.TempDir(), "cache.rkt")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	docs, err := readIndexCache(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 || docs[0].Body != "Hello\nWorld" || docs[0].Title != "Téo" || docs[1].ID != "78" {
		t.Fatalf("read Racket cache = %+v", docs)
	}
}

func TestMalformedCache(t *testing.T) {
	for _, invalid := range []string{"not racket", `(#hasheq((id . "test")`, `(#hasheq((unknown . "foo")))`, `(#hasheq((id . "bad\q")))`} {
		if _, err := readRacketCache(invalid); err == nil {
			t.Errorf("accepted malformed cache %q", invalid)
		}
	}
}
