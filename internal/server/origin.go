package server

import (
	"net/http"
	"net/url"
	"strings"
)

// allowedPOSTOrigin checks the complete authority rather than a string prefix.
// When canonical_origin is unset (the default), the public request Host is
// still authoritative. This is important for deployments behind reverse
// proxies where the internal listener uses HTTP but the public site uses HTTPS.
func (s *Server) allowedPOSTOrigin(r *http.Request, supplied string) bool {
	if supplied == "" {
		return false
	}
	u, err := url.Parse(supplied)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	expectedHost := r.Host
	expectedScheme := ""
	if canonical := s.Config.Get("canonical_origin"); canonical != "" {
		v, err := url.Parse(canonical)
		if err != nil || v.Host == "" {
			return false
		}
		expectedHost, expectedScheme = v.Host, v.Scheme
	}
	return strings.EqualFold(u.Host, expectedHost) && (expectedScheme == "" || u.Scheme == expectedScheme)
}
