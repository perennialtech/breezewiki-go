package server

import (
	"strings"
	"testing"
)

func TestSearchSuggestionsDisabledInRenderedServerPage(t *testing.T) {
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false", "feature_search_suggestions": "false"}, upstreamAPI)
	res := response(s, "GET", "/test/wiki/Example", nil, nil)
	if res.Code != 200 {
		t.Fatalf("status %d", res.Code)
	}
	if strings.Contains(res.Body.String(), `src="/static/search-suggestions.js"`) {
		t.Fatal("suggestions script still enabled")
	}
}
