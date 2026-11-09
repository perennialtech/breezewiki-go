package server

import (
	"strings"
	"testing"
)

func TestHomeContentAndPreferences(t *testing.T) {
	s, _ := mockServer(t, map[string]string{
		"canonical_origin": "https://wiki.example.com",
		"application_name": "GoWiki",
	}, upstreamAPI)
	response := response(s, "GET", "/", nil, nil)
	if response.Code != 200 {
		t.Fatalf("HTTP %d", response.Code)
	}
	body := response.Body.String()
	for _, want := range []string{"Testimonials", "mirrors", "wiki.example.com", "GoWiki source code", "Find a page", "Miraheze", "Fandom"} {
		if !strings.Contains(body, want) {
			t.Errorf("home missing %q", want)
		}
	}
}
