package server

import (
	"net/url"
	"testing"
)

func TestSettingsRedirectMustBeLocal(t *testing.T) {
	s, _ := mockServer(t, nil, upstreamAPI)
	for _, next := range []string{"https://evil.invalid/", "//evil.invalid", "\\\\evil.invalid", "", "/foo\\r\\nLocation:evil.invalid"} {
		path := "/set-user-settings?next_location=" + url.QueryEscape(next) + "&new_settings=" + url.QueryEscape("#s(user-cookies^ dark (minecraft))")
		r := response(s, "GET", path, nil, nil)
		if r.Code == 302 {
			t.Errorf("accepted redirect to %q", next)
		}
	}
	good := "/set-user-settings?next_location=" + url.QueryEscape("/minecraft/wiki/Bricks?redirect=no") + "&new_settings=" + url.QueryEscape("#s(user-cookies^ dark (minecraft))")
	r := response(s, "GET", good, nil, nil)
	if r.Code != 302 || r.Header().Get("Location") != "/minecraft/wiki/Bricks?redirect=no" {
		t.Fatalf("valid settings return: %d %s", r.Code, r.Header().Get("Location"))
	}
	if len(r.Result().Cookies()) != 2 || r.Result().Cookies()[0].Value != "dark" || r.Result().Cookies()[1].Value != "minecraft" {
		t.Fatalf("did not write expected cookies: %+v", r.Result().Cookies())
	}
}
