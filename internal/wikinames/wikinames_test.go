package wikinames

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParsePage(t *testing.T) {
	page := `<html><div class="mw-spcontent"><ul>
	<li><a href="https://terra-hexalis.fandom.com/">Terra Hexalis Wiki</a> en </li>
	<li><a href="https://fr.fandom.com/">Fr Wiki</a> fr </li>
	</ul><a class="mw-nextlink" href="?limit=5000&amp;offset=5000">next</a></div></html>`
	items, next, err := ParsePage(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0] != (Wiki{"Terra Hexalis Wiki", "https://terra-hexalis.fandom.com/", "en"}) || items[1].Lang != "fr" || next != "5000" {
		t.Fatalf("items=%+v, next=%q", items, next)
	}
}

func TestDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("offset") {
		case "":
			fmt.Fprint(w, `<div class="mw-spcontent"><li><a href="https://first.fandom.com/">First</a> en</li><a class="mw-nextlink" href="?offset=1">Next</a></div>`)
		case "1":
			fmt.Fprint(w, `<div class="mw-spcontent"><li><a href="https://second.fandom.com/">Second</a> es</li></div>`)
		default:
			t.Errorf("unexpected offset %q", r.URL.Query().Get("offset"))
		}
	}))
	defer server.Close()
	items, err := Download(context.Background(), server.Client(), server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Title != "First" || items[1].Lang != "es" {
		t.Fatalf("got %+v", items)
	}
}
