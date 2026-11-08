package server

import (
	"compress/gzip"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

var cssURL = regexp.MustCompile(`url\(([^)]*)\)`)
var cacheBuster = regexp.MustCompile(`cb=[0-9]+&?`)
var imageHash = regexp.MustCompile(`^[0-9a-f]{40}`)

// filenameSet matches the original archiver's file-name percent encoding.
const filenameSet = "<>:\"/\\|?*#~&"

func basenameFromPage(page string) string {
	segments := strings.Split(page, "/")
	for i, segment := range segments {
		var b strings.Builder
		for _, ch := range []byte(segment) {
			if ch <= 31 || ch > 126 || strings.ContainsRune(filenameSet, rune(ch)) {
				fmt.Fprintf(&b, "%%%02X", ch)
			} else {
				b.WriteByte(ch)
			}
		}
		segments[i] = b.String()
	}
	return strings.Join(segments, "#")
}
func archiveKey(basename string) string {
	if len([]rune(basename)) > 240 {
		return fmt.Sprintf("%x", sha1.Sum([]byte(basename)))
	}
	return basename
}
func (s *Server) offline(w http.ResponseWriter, r *http.Request, wiki, page string) bool {
	basename := basenameFromPage(page)
	format := strings.TrimPrefix(s.Config.Get("feature_offline::format"), ".")
	if format != "json" && format != "json.gz" {
		s.internalError(w, fmt.Errorf("unknown archive format %q", format))
		return true
	}
	filename := filepath.Join(s.ArchiveDir, wiki, archiveKey(basename)+"."+format)
	info, err := os.Stat(filename)
	if err == nil && info.Mode().IsRegular() {
		file, err := os.Open(filename)
		if err != nil {
			s.internalError(w, err)
			return true
		}
		defer file.Close()
		var reader io.Reader = file
		if format == "json.gz" {
			gz, err := gzip.NewReader(file)
			if err != nil {
				s.internalError(w, err)
				return true
			}
			defer gz.Close()
			reader = gz
		}
		var data map[string]any
		if err := json.NewDecoder(reader).Decode(&data); err != nil {
			s.internalError(w, err)
			return true
		}
		raw := fandom.ParseText(data)
		// File content is stored as parsed article JSON, exactly like a
		// successful MediaWiki parse response.
		title := fandom.String(data, "parse", "title")
		if title == "" {
			title = strings.ReplaceAll(page, "_", " ")
		}
		source := sourceURL(wiki, page)
		s.page(w, markArchived(r), wiki, title, source, raw, fandom.String(data, "parse", "headhtml"), s.siteInfo(r.Context(), wiki))
		return true
	}
	if s.DB != nil {
		var dest sql.NullString
		err := s.DB.QueryRowContext(r.Context(), "select redirect from page where wikiname = ? and basename = ?", wiki, basename).Scan(&dest)
		if err == nil && dest.Valid && dest.String != "" {
			s.redirect(w, "/"+wiki+"/wiki/"+strings.ReplaceAll(dest.String, "#", "/"))
			return true
		}
		if err != nil && err != sql.ErrNoRows {
			s.internalError(w, err)
			return true
		}
	}
	if s.Config.True("feature_offline::only") {
		s.unavailable(w, r, wiki, page, sourceURL(wiki, page))
		return true
	}
	return false
}
func (s *Server) offlineSiteInfo(ctx context.Context, wiki string) map[string]any {
	if s.DB == nil {
		return fandom.DefaultSiteInfo
	}
	var name, base, text, license sql.NullString
	err := s.DB.QueryRowContext(ctx, "select sitename, basepage, license_text, license_url from wiki where wikiname = ?", wiki).Scan(&name, &base, &text, &license)
	if err != nil {
		return fandom.DefaultSiteInfo
	}
	n, b, t, l := "Unknown Wiki", "Main_Page", "CC-BY-SA", "https://www.fandom.com/licensing"
	if name.Valid {
		n = name.String
	}
	if base.Valid {
		b = base.String
	}
	if text.Valid {
		t = text.String
	}
	if license.Valid {
		l = license.String
	}
	return map[string]any{"query": map[string]any{
		"general":    map[string]any{"sitename": n, "base": "/wiki/" + b},
		"rightsinfo": map[string]any{"text": t, "url": l},
	}}
}
func (s *Server) unavailable(w http.ResponseWriter, r *http.Request, wiki, page, source string) {
	p := html.EscapeString(r.URL.RequestURI())
	content := "<div class=\"unsaved-page\"><p>breezewiki.com doesn't have this page saved.</p><p>You can see this page by visiting a BreezeWiki mirror:</p><ul>" +
		"<li><a href=\"https://antifandom.com" + p + "\">View on antifandom.com</a></li>" +
		"<li><a href=\"https://bw.artemislena.eu" + p + "\">View on artemislena.eu</a></li>" +
		"<li><a href=\"" + quote(source) + "\">or, you can see the original page on Fandom (ugh)</a></li></ul>" +
		"<p>If you'd like " + quote(wiki) + ".fandom.com to be added to breezewiki.com, <a href=\"https://lists.sr.ht/~cadence/breezewiki-requests\">let me know about it!</a></p></div>"
	s.layout(w, markArchived(r), wiki, strings.ReplaceAll(page, "_", " "), source, content, "", s.siteInfo(r.Context(), wiki))
}

func (s *Server) archiveAsset(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/archive/") {
		return false
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/archive/"), "/", 3)
	if len(parts) != 3 || !fandom.ValidWiki(parts[0]) {
		return false
	}
	wiki, kind, dest := parts[0], parts[1], parts[2]
	if dest == "" || strings.Contains(dest, "/") || strings.Contains(dest, "\\") || dest == "." || dest == ".." {
		return false
	}
	switch kind {
	case "styles":
		filename := filepath.Join(s.ArchiveDir, wiki, "styles", dest)
		blob, err := os.ReadFile(filename)
		if err != nil {
			return false
		}
		css := cssURL.ReplaceAllStringFunc(string(blob), func(whole string) string {
			m := cssURL.FindStringSubmatch(whole)
			raw := strings.Trim(m[1], "\"'")
			for _, term := range []string{"/resources-ucp/", "/fonts/", "/drm_fonts/", "//db.onlinewebfonts.com/", "//bits.wikimedia.org/", "mygamercard.net/", "dropbox", "only=styles", "https://https://"} {
				if strings.Contains(raw, term) {
					return whole
				}
			}
			if raw == "" || raw == "'" || strings.HasPrefix(raw, "%20") || strings.HasPrefix(raw, "'") || strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "file:") {
				return whole
			}
			switch {
			case strings.HasPrefix(raw, "http://"):
				raw = "https://" + strings.TrimPrefix(raw, "http://")
			case strings.HasPrefix(raw, "httpshttps://"):
				raw = "https://" + strings.TrimPrefix(raw, "httpshttps://")
			case strings.HasPrefix(raw, "//"):
				raw = "https:" + raw
			case strings.HasPrefix(raw, "/"):
				raw = "https://" + wiki + ".fandom.com" + raw
			case !strings.HasPrefix(raw, "https://"):
				return whole
			}
			clean := cacheBuster.ReplaceAllString(raw, "")
			clean = strings.TrimRight(clean, "?&")
			hash := fmt.Sprintf("%x", sha1.Sum([]byte(clean)))
			return "url(/archive/" + wiki + "/images/" + hash + ")"
		})
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, css+"\n")
		return true
	case "images":
		if len(dest) < 40 || !imageHash.MatchString(dest) {
			return false
		}
		dir := filepath.Join(s.ArchiveDir, wiki, "images", dest[:1], dest[:2])
		list, err := os.ReadDir(dir)
		if err != nil {
			return false
		}
		for _, entry := range list {
			if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), dest) {
				continue
			}
			filename := filepath.Join(dir, entry.Name())
			if t := assetMIME(filename); t != "" {
				w.Header().Set("Content-Type", t)
			}
			w.Header().Set("Referrer-Policy", "same-origin")
			http.ServeFile(w, r, filename)
			return true
		}
	}
	return false
}
