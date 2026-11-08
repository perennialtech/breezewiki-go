package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/perennialtech/breezewiki-go/internal/archive"
)

func TestDesktopControlWorkflow(t *testing.T) {
	a, err := archive.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	queue, err := archive.NewQueue(a)
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	app := httptest.NewServer(handler(queue))
	defer app.Close()
	response, err := http.Get(app.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(body), "Reset pages") {
		t.Fatalf("dashboard: %d, %v", response.StatusCode, err)
	}
	post := func(action, wiki, origin string) int {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, app.URL+"/action", strings.NewReader(url.Values{"action": {action}, "wiki": {wiki}}.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Accept", "application/json")
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		res, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		io.Copy(io.Discard, res.Body)
		return res.StatusCode
	}
	if got := post("add", "demo", ""); got != http.StatusNoContent {
		t.Fatalf("add: %d", got)
	}
	if got := post("add", "invalid/name", ""); got != http.StatusConflict {
		t.Fatalf("invalid add: %d", got)
	}
	if got := post("add", "another", "https://evil.example"); got != http.StatusForbidden {
		t.Fatalf("cross-origin action: %d", got)
	}
	if got := post("reset", "demo", ""); got != http.StatusNoContent {
		t.Fatalf("reset: %d", got)
	}
	if got := post("reset-pages", "demo", ""); got != http.StatusNoContent {
		t.Fatalf("reset pages: %d", got)
	}
	res, err := http.Get(app.URL + "/state")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var items []archive.QueueItem
	if err := json.NewDecoder(res.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Wiki != "demo" || items[0].State != "queued" {
		t.Fatalf("queue state: %+v", items)
	}
}
