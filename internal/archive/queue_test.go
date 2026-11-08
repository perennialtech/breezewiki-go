package archive

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueuePauseResumeReset(t *testing.T) {
	var started = make(chan struct{}, 1)
	var parseRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api.php":
			if r.URL.Query().Get("action") == "parse" {
				if parseRequests.Add(1) == 1 {
					started <- struct{}{}
					<-r.Context().Done()
					return
				}
				fmt.Fprint(w, `{"parse":{"title":"Page","pageid":1,"text":"<p>Body</p>"}}`)
			} else {
				http.Error(w, "unexpected query", http.StatusNotFound)
			}
		case "/wikia.php", "/load.php":
			fmt.Fprint(w, "/* empty */")
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	a, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := a.DB.Exec(`insert into wiki(wikiname,progress) values('demo',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(`insert into page(wikiname,basename,progress) values('demo','Page',0)`); err != nil {
		t.Fatal(err)
	}
	a.Origin = func(string) string { return server.URL }
	q, err := NewQueue(a)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	if items := q.Snapshot(); len(items) != 1 || items[0].Wiki != "demo" || items[0].Stage != 1 || items[0].State != "queued" {
		t.Fatalf("restored pending wiki: %+v", items)
	}
	if err := q.Resume("demo"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("page request never started")
	}
	if err := q.Pause("demo"); err != nil {
		t.Fatal(err)
	}
	if state := q.Snapshot()[0].State; state != "paused" {
		t.Fatalf("paused state = %s", state)
	}
	var progress int
	if err := a.DB.QueryRow(`select progress from page where wikiname='demo' and basename='Page'`).Scan(&progress); err != nil || progress != 0 {
		t.Fatalf("page progress after cancel: %d, %v", progress, err)
	}
	if err := q.Resume("demo"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(4 * time.Second)
	for {
		if state := q.Snapshot()[0].State; state == "complete" {
			break
		} else if state == "error" {
			t.Fatalf("queue error: %+v", q.Snapshot())
		}
		select {
		case <-deadline:
			t.Fatalf("queue never completed: %+v", q.Snapshot())
		default:
			time.Sleep(time.Millisecond * 10)
		}
	}
	if got := parseRequests.Load(); got != 2 {
		t.Fatalf("expected one canceled and one completed parse request, got %d", got)
	}
	if _, err := os.Stat(PagePath(a.Storage, "demo", "Page", ".json")); err != nil {
		t.Fatal(err)
	}
	if err := q.Reset("demo", false); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.QueryRow(`select progress from wiki where wikiname='demo'`).Scan(&progress); err != nil || progress != 0 {
		t.Fatalf("reset wiki progress: %d, %v", progress, err)
	}
	if err := a.DB.QueryRow(`select progress from page where wikiname='demo' and basename='Page'`).Scan(&progress); err != nil || progress != 1 {
		t.Fatalf("reset must preserve page progress: %d, %v", progress, err)
	}
	if err := q.Reset("demo", true); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := a.DB.QueryRow(`select count(*) from page where wikiname='demo'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("reset pages must delete the page list: %d, %v", count, err)
	}
	if items := q.Snapshot(); items[0].State != "queued" || items[0].Stage != 0 {
		t.Fatalf("reset queue state: %+v", items)
	}
	if err := q.Add("invalid/name"); err == nil {
		t.Fatal("accepted invalid wiki name")
	}
	if err := q.Add("other"); err != nil {
		t.Fatal(err)
	}
	if err := q.Add("other"); err != nil {
		t.Fatal(err)
	}
	if len(q.Snapshot()) != 2 {
		t.Fatalf("duplicate queue entry: %+v", q.Snapshot())
	}
}

func TestExtractUnicode(t *testing.T) {
	content, _ := json.Marshal(map[string]any{"parse": map[string]any{"title": "日本語", "pageid": 4, "text": "<p>日本語 &amp; text</p>"}})
	doc, err := ExtractDocument(content)
	if err != nil || doc.Len != len([]rune(doc.Body)) {
		t.Fatalf("unicode document = %+v, %v", doc, err)
	}
}
