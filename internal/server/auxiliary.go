package server

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

var settingsRE = regexp.MustCompile(`^#s\(user-cookies\^\s+(default|light|dark)\s+\(([^)]*)\)\)$`)

// localReturnPath prevents settings changes from redirecting visitors to
// attacker-controlled domains.
func localReturnPath(path string) bool {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "\r\n\\") {
		return false
	}
	u, err := url.ParseRequestURI(path)
	return err == nil && u.Host == "" && !u.IsAbs()
}

func (s *Server) setUserSettings(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Query().Get("next_location")
	setting := r.URL.Query().Get("new_settings")
	m := settingsRE.FindStringSubmatch(setting)
	if m == nil || !localReturnPath(next) {
		s.internalError(w, fmt.Errorf("invalid user settings %q", setting))
		return
	}
	notices := strings.Fields(m[2])
	age := int(60 * 60 * 24 * 365 * 10)
	for _, c := range []*http.Cookie{
		{Name: "theme", Value: m[1], Path: "/", MaxAge: age},
		{Name: "notices", Value: strings.Join(notices, "|"), Path: "/", MaxAge: age},
	} {
		http.SetCookie(w, c)
	}
	s.redirect(w, next)
}
func (s *Server) proxy(w http.ResponseWriter, r *http.Request) {
	dest := r.URL.Query().Get("dest")
	if !fandom.AllowedProxyURL(dest) {
		s.notFound(w, r)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, dest, nil)
	if err != nil {
		s.internalError(w, err)
		return
	}
	client := s.Fandom.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		s.internalError(w, err)
		return
	}
	defer res.Body.Close()
	s.common(w)
	if ct := res.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(res.StatusCode)
	if _, err := io.Copy(w, res.Body); err != nil {
		fmt.Printf("proxy copy failed: %v\n", err)
	}
}
