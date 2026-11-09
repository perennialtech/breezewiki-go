// Package server serves BreezeWiki's HTTP routes and offline assets.
package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/perennialtech/breezewiki-go"
	"github.com/perennialtech/breezewiki-go/internal/config"
	"github.com/perennialtech/breezewiki-go/internal/fandom"
	"github.com/perennialtech/breezewiki-go/internal/render"
)

var wikiPath = regexp.MustCompile(`^/([a-zA-Z0-9-]{1,50})(?:/(.*))?$`)
var captchaImagePath = regexp.MustCompile(`^/captcha/img/([0-9]+)/([0-9]+)$`)
var captchaVerifyPath = regexp.MustCompile(`^/captcha/verify/([0-9]+)/([0-9]+)/([0-9]+)/([0-9]+)$`)
var redirectMessage = regexp.MustCompile(`(?is)<[^>]*class=["'][^"']*\bredirectMsg\b[^"']*["'][^>]*>.*?<a\b[^>]*href=["']([^"']+)["']`)

type Server struct {
	Config      *config.Config
	Fandom      *fandom.Client
	StaticDir   string
	ArchiveDir  string
	SolrBaseURL string
	// DB is an optional database/sql-compatible connection to storage/archiver.db.
	// When present, offline routes use the original wiki and page redirect tables.
	DB *sql.DB
}

// New serves embedded assets by default. Set StaticDir to override them from disk.
func New(cfg *config.Config, client *fandom.Client) *Server {
	if cfg == nil {
		cfg = config.New(nil)
	}
	if client == nil {
		client = fandom.New(cfg)
	}
	return &Server{Config: cfg, Fandom: client, ArchiveDir: filepath.Join("storage", "archive")}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.Config.True("access_log::enabled") {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.RequestURI())
	}
	s.serve(w, r)
}
func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if s.subdomain(w, r) {
		return
	}
	uri := r.URL.Path
	// Upstream fixes semicolon parsing, treating each semicolon as page text.
	if uri == "/buddyfight/wiki/It_Doesn't_Work!!" && r.URL.Query().Has("b") {
		s.redirect(w, "/stampylongnose/wiki/It_Works")
		return
	}
	if uri == "/" {
		s.home(w)
		return
	}
	if uri == "/robots.txt" || strings.HasPrefix(uri, "/static/") {
		s.static(w, r, uri)
		return
	}
	if m := captchaImagePath.FindStringSubmatch(uri); m != nil {
		s.captchaImage(w, r, m)
		return
	}
	if m := captchaVerifyPath.FindStringSubmatch(uri); m != nil {
		s.captchaVerify(w, r, m)
		return
	}
	if s.Config.True("captcha::enabled") && !strings.HasPrefix(uri, "/captcha/") {
		if _, err := r.Cookie("captcha"); err != nil {
			s.captcha(w, r)
			return
		}
	}
	switch uri {
	case "/proxy":
		s.proxy(w, r)
		return
	case "/search":
		s.globalSearch(w, r)
		return
	case "/set-user-settings":
		s.setUserSettings(w, r)
		return
	}
	if m := wikiPath.FindStringSubmatch(uri); m != nil {
		wiki, rest := m[1], m[2]
		switch {
		case strings.HasPrefix(rest, "wiki/Category:") && len(rest) > len("wiki/Category:"):
			s.article(w, r, wiki, strings.TrimPrefix(rest, "wiki/"), "category")
			return
		case strings.HasPrefix(rest, "wiki/File:") && len(rest) > len("wiki/File:"):
			s.article(w, r, wiki, strings.TrimPrefix(rest, "wiki/"), "file")
			return
		case strings.HasPrefix(rest, "wiki/") && len(rest) > len("wiki/"):
			page := strings.TrimPrefix(rest, "wiki/")
			if s.Config.True("feature_offline::enabled") && s.offline(w, r, wiki, page) {
				return
			}
			s.article(w, r, wiki, page, "wiki")
			return
		case rest == "search":
			s.article(w, r, wiki, "", "search")
			return
		case rest == "" || rest == "wiki" || rest == "wiki/":
			s.redirectWiki(w, r, wiki)
			return
		}
	}
	if strings.HasPrefix(uri, "/archive/") {
		if s.Config.True("feature_offline::enabled") && s.archiveAsset(w, r) {
			return
		}
		s.static(w, r, uri)
		return
	}
	s.notFound(w, r)
}
func (s *Server) common(w http.ResponseWriter) {
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Link", fmt.Sprintf("<%s>; rel=preload; as=style, <%s>; rel=preload; as=script, </static/source-sans-pro-v21-vietnamese_latin-ext_latin_greek-ext_greek_cyrillic-ext_cyrillic-regular.woff2>; rel=preload; as=font; crossorigin; type=font/woff2",
		s.staticURL("main.css"), s.staticURL("preact.js")))
}
func (s *Server) staticURL(name string) string {
	if s.StaticDir != "" {
		p := filepath.Join(s.StaticDir, name)
		if st, err := os.Stat(p); err == nil {
			return "/static/" + name + "?t=" + strconv.FormatInt(st.ModTime().Unix(), 10)
		}
		return "/static/" + name
	}
	data, err := fs.ReadFile(breezewiki.StaticFiles(), name)
	if err != nil {
		return "/static/" + name
	}
	digest := sha256.Sum256(data)
	return fmt.Sprintf("/static/%s?t=%x", name, digest[:8])
}
func (s *Server) html(w http.ResponseWriter, status int, body string) {
	s.common(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}
func (s *Server) internalError(w http.ResponseWriter, err error) {
	log.Printf("server error: %v", err)
	s.html(w, 500, "<!doctype html><html><body><h1>Internal server error</h1><pre>"+html.EscapeString(err.Error())+"</pre></body></html>")
}
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.html(w, 404, "<html><body><h1>Not found.</h1><pre>"+html.EscapeString(r.Method+" "+r.URL.RequestURI())+"</pre></body></html>")
}
func (s *Server) redirect(w http.ResponseWriter, dest string) {
	s.common(w)
	w.Header().Set("Location", dest)
	s.html(w, 302, "<html><head><title>Redirecting...</title></head><body>Redirecting to <a href=\""+html.EscapeString(dest)+"\">"+html.EscapeString(dest)+"</a>...</body></html>")
}
func (s *Server) subdomain(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("X-Canonical-Origin")
	if origin == "" {
		origin = s.Config.Get("canonical_origin")
	}
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := r.Host
	if strings.HasSuffix(host, "."+u.Hostname()) {
		wiki := strings.TrimSuffix(host, "."+u.Hostname())
		if !strings.Contains(wiki, ".") && wiki != "" {
			// Upstream does not carry the query string to this redirect.
			s.redirect(w, strings.TrimRight(origin, "/")+"/"+wiki+"/"+strings.TrimPrefix(r.URL.Path, "/"))
			return true
		}
	}
	return false
}
func (s *Server) redirectWiki(w http.ResponseWriter, r *http.Request, wiki string) {
	info := s.siteInfo(r.Context(), wiki)
	_, base := fandom.General(info)
	s.redirect(w, "/"+wiki+"/wiki/"+base)
}
func (s *Server) globalSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	wiki := q.Get("wikiname")
	if wiki == "" {
		s.common(w)
		http.Error(w, "Requires wikiname and q parameters.", 400)
		return
	}
	if !fandom.ValidWiki(wiki) {
		s.notFound(w, r)
		return
	}
	if q.Get("q") == "" {
		s.redirectWiki(w, r, wiki)
		return
	}
	s.redirect(w, "/"+wiki+"/search?q="+url.QueryEscape(q.Get("q")))
}
func (s *Server) page(w http.ResponseWriter, r *http.Request, wiki, title, source, raw, head string, siteinfo map[string]any) {
	rewritten, err := rewriteContent(raw, wiki, s.Config.True("strict_proxy"))
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.layout(w, r, wiki, title, source, rewritten, head, siteinfo)
}
func (s *Server) layout(w http.ResponseWriter, r *http.Request, wiki, title, source, content, head string, siteinfo map[string]any) {
	out, err := s.pageHTML(r, wiki, title, source, content, head, siteinfo)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if r.URL.Query().Get("redirect") != "no" {
		if m := redirectMessage.FindStringSubmatch(out); m != nil {
			w.Header().Set("Refresh", "0;url="+html.UnescapeString(m[1]))
		}
	}
	s.html(w, 200, out)
}
func (s *Server) pageHTML(r *http.Request, wiki, title, source, content, head string, siteinfo map[string]any) (string, error) {
	if siteinfo == nil {
		siteinfo = fandom.DefaultSiteInfo
	}
	origin := s.Config.Get("canonical_origin")
	out, err := render.Page(content, render.Options{Wiki: wiki, Title: title, SourceURL: source,
		CanonicalOrigin: origin, ApplicationName: s.Config.Get("application_name"),
		Theme: theme(r), SiteInfo: siteinfo, HeadHTML: head, Path: r.URL.Path,
		CurrentURL: r.URL.RequestURI(), Notices: notices(r)})
	if err != nil {
		return "", err
	}
	return s.adjustPageAssets(out, r, wiki), nil
}
func rewriteContent(raw, wiki string, strict bool) (string, error) {
	if strict {
		return render.RewriteWithProxy(raw, wiki, true)
	}
	return render.Rewrite(raw, wiki)
}
func theme(r *http.Request) string {
	c, err := r.Cookie("theme")
	if err != nil {
		return "default"
	}
	switch c.Value {
	case "light", "dark", "default":
		return c.Value
	}
	return "default"
}
func notices(r *http.Request) []string {
	cookie, err := r.Cookie("notices")
	if err != nil || cookie.Value == "" {
		return nil
	}
	return strings.Split(cookie.Value, "|")
}

func (s *Server) siteInfo(ctx context.Context, wiki string) map[string]any {
	if s.Config.True("feature_offline::only") {
		return s.offlineSiteInfo(ctx, wiki)
	}
	return s.Fandom.SiteInfo(ctx, wiki)
}
func (s *Server) static(w http.ResponseWriter, r *http.Request, uri string) {
	root := s.StaticDir
	rel := ""
	if uri == "/robots.txt" {
		rel = "robots.txt"
	} else if strings.HasPrefix(uri, "/static/") {
		rel = strings.TrimPrefix(uri, "/static/")
	} else if strings.HasPrefix(uri, "/archive/") {
		root = s.ArchiveDir
		rel = strings.TrimPrefix(uri, "/archive/")
	} else {
		s.notFound(w, r)
		return
	}
	if rel == "" || strings.Contains(rel, "\\") || strings.Contains(rel, "..") {
		s.notFound(w, r)
		return
	}
	var embedded fs.FS
	if root == "" {
		embedded = breezewiki.StaticFiles()
		info, err := fs.Stat(embedded, rel)
		if err != nil || !info.Mode().IsRegular() {
			s.notFound(w, r)
			return
		}
	} else {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || !info.Mode().IsRegular() {
			s.notFound(w, r)
			return
		}
	}
	s.common(w)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Referrer-Policy", "same-origin")
	if s.Config.True("debug") {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	}
	if ct := assetMIME(rel); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if embedded != nil {
		http.ServeFileFS(w, r, embedded, rel)
	} else {
		http.ServeFile(w, r, filepath.Join(root, filepath.FromSlash(rel)))
	}
}

func assetMIME(filename string) string {
	switch filepath.Ext(filename) {
	case ".css":
		return "text/css"
	case ".js":
		return "text/javascript"
	case ".txt":
		return "text/plain"
	case ".woff2":
		return "font/woff2"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	}
	return mime.TypeByExtension(filepath.Ext(filename))
}
func sourceURL(wiki, page string) string { return "https://" + wiki + ".fandom.com/wiki/" + page }
func quote(v string) string              { return html.EscapeString(v) }
func linkTo(wiki, title string) string {
	// MediaWiki page titles encode slashes as path separators.
	p := strings.ReplaceAll(title, " ", "_")
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		segments[i] = pathEscape(seg)
	}
	return "/" + wiki + "/wiki/" + strings.Join(segments, "/")
}
func pathEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c <= 0x1f || c > 0x7e || strings.ContainsRune(";?`{} \"#<>", rune(c)) {
			fmt.Fprintf(&b, "%%%02X", c)
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}
func jsonString(v any) string                         { b, _ := json.Marshal(v); return string(b) }
func requestParam(r *http.Request, key string) string { return r.URL.Query().Get(key) }
func wrapError(where string, err error) error         { return fmt.Errorf("%s: %w", where, err) }
