package fandomhttp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("X-Request-Test") != "yes" {
			t.Errorf("unexpected request: %s %s", r.Method, r.Header)
		}
		w.Header().Set("X-Response-Test", "ok")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	defer srv.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Request-Test", "yes")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated || resp.Header.Get("X-Response-Test") != "ok" || string(body) != "created" {
		t.Errorf("response: code=%d headers=%v body=%q", resp.StatusCode, resp.Header, body)
	}
}
