package server

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/perennialtech/breezewiki-go/internal/config"
)

func TestArchiveFilenamesAndDatabase(t *testing.T) {
	if got := basenameFromPage("Category:Foo/A&B"); got != "Category%3AFoo#A%26B" {
		t.Fatalf("archive filename: %q", got)
	}
	if got := archiveKey(strings.Repeat("a", 241)); len(got) != 40 {
		t.Fatalf("long archive filename: %q", got)
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec("create table page (wikiname text, basename text, redirect text);create table wiki (wikiname text, sitename text, basepage text, license_text text, license_url text)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("insert into page values ('test','Alias','Target#Subpage');insert into wiki values ('test','Example','Portal','CC','https://license.test')")
	if err != nil {
		t.Fatal(err)
	}
	s := New(config.New(map[string]string{"feature_offline::enabled": "true", "feature_offline::only": "true"}), nil)
	s.ArchiveDir = t.TempDir()
	s.DB = db
	got := response(s, "GET", "/test/wiki/Alias", nil, nil)
	if got.Code != 302 || got.Header().Get("Location") != "/test/wiki/Target/Subpage" {
		t.Fatalf("archive redirect %d %v", got.Code, got.Header())
	}
	got = response(s, "GET", "/test", nil, nil)
	if got.Code != 302 || got.Header().Get("Location") != "/test/wiki/Portal" {
		t.Fatalf("offline siteinfo base %d %v", got.Code, got.Header())
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProxyDestinationAndHeaders(t *testing.T) {
	s := New(config.New(nil), nil)
	s.Fandom.HTTP = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "static.wikia.nocookie.net" {
			t.Errorf("unexpected proxy host %q", r.URL.Host)
		}
		return &http.Response{StatusCode: 206, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader("mock-png")), Request: r}, nil
	})}
	got := response(s, "GET", "/proxy?dest=https%3A%2F%2Fstatic.wikia.nocookie.net%2Ftest%2Fimage.png", nil, nil)
	if got.Code != 206 || got.Body.String() != "mock-png" || got.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("proxy response %d %v %q", got.Code, got.Header(), got.Body.String())
	}
	got = response(s, "GET", "/proxy?dest=https%3A%2F%2Fevil.example%2F", nil, nil)
	if got.Code != 404 {
		t.Fatalf("unapproved proxy status %d", got.Code)
	}
}
func TestCaptchaCoordinate(t *testing.T) {
	s := New(config.New(nil), nil)
	req := httptest.NewRequest("GET", "/captcha/img/240/120", nil)
	x, y := s.captchaLocation(req, 240, 120)
	got := response(s, "GET", fmt.Sprintf("/captcha/verify/240/120/%d/%d?from=%%2Ftest", x, y), nil, nil)
	if got.Code != 302 || len(got.Result().Cookies()) != 1 || got.Result().Cookies()[0].Value != "coordinate" {
		t.Fatalf("captcha coordinate %d %v", got.Code, got.Header())
	}
}
