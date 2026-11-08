package archive

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestStagesAndResumeWithRealSQLiteAndHTTP(t *testing.T) {
	longTitle := strings.Repeat("Long", 65)
	var mu sync.Mutex
	requests := map[string]int{}
	var posted [][]Document
	var server *httptest.Server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.URL.Path+"?"+r.URL.Query().Get("action")+":"+r.URL.Query().Get("page")]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api.php":
			switch r.URL.Query().Get("action") {
			case "query":
				switch r.URL.Query().Get("meta") {
				case "siteinfo":
					fmt.Fprint(w, `{"query":{"general":{"sitename":"Demo Wiki","base":"https://demo.fandom.com/wiki/Main_Page"},"rightsinfo":{"text":"CC-BY-SA","url":"https://license"},"statistics":{"articles":2},"namespaces":{"0":{"id":0,"content":true},"1":{"id":1},"3000":{"id":3000,"content":true}}}}`)
					return
				}
				if r.URL.Query().Get("list") == "allpages" {
					if r.URL.Query().Get("apfilterredir") == "nonredirects" {
						if r.URL.Query().Get("apcontinue") == "" {
							fmt.Fprint(w, `{"query":{"allpages":[{"title":"Alpha"}]},"continue":{"apcontinue":"next"}}`)
						} else {
							fmt.Fprintf(w, `{"query":{"allpages":[{"title":%q}]}}`, longTitle)
						}
					} else {
						fmt.Fprint(w, `{"query":{"allpages":[{"title":"Alias"},{"title":"Gone"}]}}`)
					}
					return
				}
				if r.URL.Query().Get("prop") == "links" {
					if r.URL.Query().Get("titles") == "Alias" {
						fmt.Fprint(w, `{"query":{"pages":[{"links":[{"title":"Alpha"}]}]}}`)
					} else {
						fmt.Fprint(w, `{"query":{"pages":[{}]}}`)
					}
					return
				}
			case "parse":
				page := r.URL.Query().Get("page")
				if page == longTitle {
					mu.Lock()
					n := requests[r.URL.Path+"?parse:"+page]
					mu.Unlock()
					if n == 1 {
						http.Error(w, "temporary upstream failure", http.StatusBadGateway)
						return
					}
					fmt.Fprintf(w, `{"parse":{"title":%q,"pageid":8,"text":"<p>Second page</p>"}}`, page)
					return
				}
				fmt.Fprint(w, `{"parse":{"title":"Alpha","pageid":7,"text":"<div class=\"mw-parser-output\"><h2>Heading</h2><p>Visible &amp; welcome</p><div class=\"collapsed\">Hidden</div><ul><li>List item</li></ul><table><tr><td>Table cell</td></tr></table></div>"}}`)
				return
			}
		case "/wikia.php", "/load.php":
			w.Header().Set("Content-Type", "text/css")
			fmt.Fprintf(w, "body{background:url('%s/image.png?cb=123&width=20')}\n", server.URL)
			return
		case "/image.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("\x89PNG\r\n\x1a\nimage data"))
			return
		case "/solr/demo/update":
			if r.Method != http.MethodPost || r.URL.Query().Get("commit") != "true" || r.Header.Get("Content-Type") != "application/json" {
				http.Error(w, "unexpected update request", http.StatusBadRequest)
				return
			}
			var batch []Document
			if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			posted = append(posted, batch)
			mu.Unlock()
			fmt.Fprint(w, `{"responseHeader":{"status":0}}`)
			return
		case "/solr/demo/select":
			if r.URL.Query().Get("defType") != "edismax" || r.URL.Query().Get("q.op") != "OR" || r.URL.Query().Get("sort") != "len desc" {
				http.Error(w, "unexpected search filters", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `{"responseHeader":{"QTime":12},"response":{"numFound":1,"docs":[{"id":"7","title":["Alpha"],"len":42}]},"highlighting":{"7":{"title":["Alpha"],"body":["Visible <mark>welcome</mark>"]}}}`)
			return
		}
		http.Error(w, "unexpected endpoint: "+r.URL.String(), http.StatusNotFound)
	})
	server = httptest.NewTLSServer(handler)
	defer server.Close()
	storage := t.TempDir()
	a, err := Open(storage)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.Client = server.Client()
	a.Origin = func(wiki string) string { return server.URL }
	ctx := context.Background()
	if err := a.RunStage(ctx, "demo", 0, nil); err != nil {
		t.Fatalf("stage 1: %v", err)
	}
	var wikiStage int
	if err := a.DB.QueryRow(`select progress from wiki where wikiname='demo'`).Scan(&wikiStage); err != nil || wikiStage != 1 {
		t.Fatalf("wiki stage: %d, %v", wikiStage, err)
	}
	var pageCount int
	if err := a.DB.QueryRow(`select count(*) from page where wikiname='demo'`).Scan(&pageCount); err != nil || pageCount != 4 {
		t.Fatalf("page count: %d, %v", pageCount, err)
	}
	if err := a.RunStage(ctx, "demo", 1, nil); err == nil {
		t.Fatal("first page stage should fail on temporary API error")
	}
	var alphaProgress int
	if err := a.DB.QueryRow(`select progress from page where wikiname='demo' and basename='Alpha'`).Scan(&alphaProgress); err != nil || alphaProgress != 1 {
		t.Fatalf("first page should be durable: %d, %v", alphaProgress, err)
	}
	if err := a.RunStage(ctx, "demo", 1, nil); err != nil {
		t.Fatalf("resume page stage: %v", err)
	}
	mu.Lock()
	alphaCalls := requests["/api.php?parse:Alpha"]
	longCalls := requests["/api.php?parse:"+longTitle]
	mu.Unlock()
	if alphaCalls != 1 || longCalls != 2 {
		t.Fatalf("unexpected fetch calls after resume: Alpha=%d Long=%d", alphaCalls, longCalls)
	}
	if err := a.DB.QueryRow(`select progress from wiki where wikiname='demo'`).Scan(&wikiStage); err != nil || wikiStage != 2 {
		t.Fatalf("page stage: %d, %v", wikiStage, err)
	}
	var dest string
	if err := a.DB.QueryRow(`select redirect from page where wikiname='demo' and basename='Alias'`).Scan(&dest); err != nil || dest != "Alpha" {
		t.Fatalf("redirect: %q, %v", dest, err)
	}
	if err := a.DB.QueryRow(`select count(*) from page where wikiname='demo'`).Scan(&pageCount); err != nil || pageCount != 3 {
		t.Fatalf("deleted redirect: %d, %v", pageCount, err)
	}
	longKey := PageKey(TitleBasename(longTitle))
	if longKey == longTitle {
		t.Fatal("long page filename was not hashed")
	}
	if _, err := os.Stat(PagePath(storage, "demo", TitleBasename(longTitle), ".json")); err != nil {
		t.Fatal(err)
	}
	var special string
	if err := a.DB.QueryRow(`select basename from special_page where wikiname='demo' and key=?`, longKey).Scan(&special); err != nil || special != longTitle {
		t.Fatalf("special page entry: %q, %v", special, err)
	}
	if err := a.RunStage(ctx, "demo", 2, nil); err != nil {
		t.Fatalf("style stage: %v", err)
	}
	for _, file := range []string{"themeVariables-default.css", "themeVariables-light.css", "themeVariables-dark.css", "site.css"} {
		if _, err := os.Stat(filepath.Join(storage, "archive", "demo", "styles", file)); err != nil {
			t.Fatalf("missing style %s: %v", file, err)
		}
	}
	if err := a.DB.QueryRow(`select count(*) from image where wikiname='demo'`).Scan(&pageCount); err != nil || pageCount != 1 {
		t.Fatalf("deduplicated style images: %d, %v", pageCount, err)
	}
	if err := a.RunStage(ctx, "demo", 3, nil); err != nil {
		t.Fatalf("image stage: %v", err)
	}
	imageURL, hash := ImageKey(server.URL + "/image.png?width=20")
	var urlFromDB, ext string
	if err := a.DB.QueryRow(`select url, ext from image where wikiname='demo' and hash=?`, hash).Scan(&urlFromDB, &ext); err != nil || urlFromDB != imageURL || ext != "png" {
		t.Fatalf("saved image: %q %q %v", urlFromDB, ext, err)
	}
	if _, err := os.Stat(filepath.Join(ImageDir(storage, "demo", hash), hash+".png")); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.QueryRow(`select progress from wiki where wikiname='demo'`).Scan(&wikiStage); err != nil || wikiStage != 4 {
		t.Fatalf("complete wiki progress: %d, %v", wikiStage, err)
	}
	if pending, err := a.Pending(); err != nil || len(pending) != 0 {
		t.Fatalf("completed wiki is pending: %v, %v", pending, err)
	}
	if err := a.Run(ctx, "demo", nil); err != nil {
		t.Fatalf("idempotent rerun: %v", err)
	}
	mu.Lock()
	alphaCalls = requests["/api.php?parse:Alpha"]
	imageCalls := requests["/image.png?:"]
	mu.Unlock()
	if alphaCalls != 1 || imageCalls != 1 {
		t.Fatalf("unexpected re-download: Alpha=%d image=%d", alphaCalls, imageCalls)
	}

	// A compressed archive page remains readable to the indexer, with the
	// same Solr schema, even though the downloader writes uncompressed JSON.
	alphaPath := PagePath(storage, "demo", "Alpha", ".json")
	original, err := os.ReadFile(alphaPath)
	if err != nil {
		t.Fatal(err)
	}
	var z bytes.Buffer
	zw := gzip.NewWriter(&z)
	if _, err := zw.Write(original); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(alphaPath+".gz", z.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(alphaPath); err != nil {
		t.Fatal(err)
	}
	docs, err := a.BuildDocuments(ctx, "demo", nil)
	if err != nil || len(docs) != 2 {
		t.Fatalf("build documents: %d, %v", len(docs), err)
	}
	foundAlpha := false
	for _, doc := range docs {
		if doc.ID == "7" {
			foundAlpha = true
			if !strings.Contains(doc.Body, "Visible & welcome") || strings.Contains(doc.Body, "Table cell") || strings.Contains(doc.Body, "Hidden") || strings.Contains(doc.Body, "List item") {
				t.Errorf("wrong body extraction: %q", doc.Body)
			}
			if !strings.Contains(doc.Table, "Table cell") || !strings.Contains(doc.Table, "List item") || strings.Contains(doc.Table, "Visible & welcome") {
				t.Errorf("wrong table extraction: %q", doc.Table)
			}
			if doc.Len != utf8.RuneCountInString(doc.Body) {
				t.Errorf("incorrect Solr len: %d", doc.Len)
			}
		}
	}
	if !foundAlpha {
		t.Fatal("Alpha document missing")
	}
	cache := filepath.Join(storage, "cache.json")
	if err := a.Index(ctx, "demo", IndexOptions{CachePath: cache, SolrURL: server.URL + "/solr"}, nil); err != nil {
		t.Fatalf("index: %v", err)
	}
	if err := a.Index(ctx, "demo", IndexOptions{ReadFromCache: true, CachePath: cache, SolrURL: server.URL + "/solr"}, nil); err != nil {
		t.Fatalf("cached index: %v", err)
	}
	mu.Lock()
	postCount := len(posted)
	var postLen int
	for _, batch := range posted {
		postLen += len(batch)
	}
	mu.Unlock()
	if postCount != 2 || postLen != 4 {
		t.Errorf("Solr posting count: requests=%d docs=%d", postCount, postLen)
	}
	found, err := a.SearchSolr(ctx, server.URL+"/solr", "demo", "welcome", "or", "len")
	if err != nil || found.Found != 1 || found.QueryTimeMS != 12 || len(found.Results) != 1 || found.Results[0].Title != "Alpha" {
		t.Fatalf("Solr search: %+v, %v", found, err)
	}
}

func TestExtractDocumentRejectsInvalidFiles(t *testing.T) {
	for _, input := range [][]byte{nil, []byte(`{}`), []byte(`{"parse":{"title":"No ID","text":"<p>hi</p>"}}`)} {
		if _, err := ExtractDocument(input); err == nil {
			t.Errorf("expected bad source to fail: %q", input)
		}
	}
}

func TestSearchResultAndCacheFailure(t *testing.T) {
	a, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := a.SearchSolr(context.Background(), "", "a/b", "query", "and", "relevance"); err == nil {
		t.Fatal("invalid wiki name accepted")
	}
	if err := a.Index(context.Background(), "demo", IndexOptions{ReadFromCache: true, CachePath: filepath.Join(a.Storage, "not-found")}, nil); err == nil {
		t.Fatal("missing cache should fail")
	}
}

func TestIndexerSkipsUnreadablePages(t *testing.T) {
	storage := t.TempDir()
	a, err := Open(storage)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	dir := filepath.Join(storage, "archive", "demo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"good.json":   `{"parse":{"pageid":4,"title":"Good","text":"<p>Known content</p>"}}`,
		"broken.json": `not JSON`,
		"bad.json.gz": `not gzip`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var reports int
	docs, err := a.BuildDocuments(context.Background(), "demo", func(done, total int, title string) {
		reports++
		if total != 3 {
			t.Errorf("progress total = %d", total)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Title != "Good" || reports != 3 {
		t.Fatalf("documents %+v, reports %d", docs, reports)
	}
}

func TestCacheMissRebuildsAndPostsToSolr(t *testing.T) {
	storage := t.TempDir()
	a, err := Open(storage)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	dir := filepath.Join(storage, "archive", "demo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Page.json"), []byte(`{"parse":{"pageid":3,"title":"Page","text":"<p>Test</p>"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		if r.Method != http.MethodPost || r.URL.Path != "/solr/demo/update" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"responseHeader":{"status":0}}`))
	}))
	defer server.Close()
	cache := filepath.Join(storage, "cache.rkt")
	if err := a.Index(context.Background(), "demo", IndexOptions{ReadFromCache: true, CachePath: cache, SolrURL: server.URL + "/solr"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Errorf("cache should be built: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := a.Index(context.Background(), "demo", IndexOptions{ReadFromCache: true, CachePath: cache, SolrURL: server.URL + "/solr"}, nil); err != nil {
		t.Fatalf("cached index should not read archive: %v", err)
	}
	if posts != 2 {
		t.Errorf("expected two Solr updates, got %d", posts)
	}
}
