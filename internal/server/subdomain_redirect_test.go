package server

import (
	"testing"

	"github.com/perennialtech/breezewiki-go/internal/config"
)

func TestSubdomainRedirectPreservesEscapedPathAndQuery(t *testing.T) {
	s := New(config.New(map[string]string{"canonical_origin": "https://mirror.example"}), nil)
	for _, tc := range []struct {
		request, location string
	}{
		{"/wiki/What%27s_Buzzin%27_Buzzard%3F", "https://mirror.example/test/wiki/What%27s_Buzzin%27_Buzzard%3F"},
		{"/wiki/Page%23Part", "https://mirror.example/test/wiki/Page%23Part"},
		{"/wiki/Page%25Encoded?q=one%2Btwo", "https://mirror.example/test/wiki/Page%25Encoded?q=one%2Btwo"},
		{"/wiki/Page%2FSubpage?redirect=no&x=%23", "https://mirror.example/test/wiki/Page%2FSubpage?redirect=no&x=%23"},
	} {
		t.Run(tc.request, func(t *testing.T) {
			got := response(s, "GET", "https://test.mirror.example"+tc.request, nil, nil)
			if got.Code != 302 || got.Header().Get("Location") != tc.location {
				t.Fatalf("status %d, Location %q; want %q", got.Code, got.Header().Get("Location"), tc.location)
			}
		})
	}
}
