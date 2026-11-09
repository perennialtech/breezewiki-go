package server

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/perennialtech/breezewiki-go/internal/fandom"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

var settingsRE = regexp.MustCompile(`^#s\(user-cookies\^\s+(default|light|dark)\s+\(([^)]*)\)\)$`)
var ipDigits = regexp.MustCompile(`[0-9]+`)

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
func (s *Server) clientIP(r *http.Request) string {
	if h := s.Config.Get("captcha::ip_header"); h != "" {
		if v := r.Header.Get(h); v != "" {
			return v
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
func (s *Server) captchaRNG(r *http.Request) *rand.Rand {
	ip := s.clientIP(r)
	var seed int64
	for i, seg := range ipDigits.FindAllString(ip, -1) {
		n, _ := strconv.ParseInt(seg, 10, 64)
		seed += int64(i+1) * (n + 1)
	}
	seed &= 0xffffffff
	return rand.New(rand.NewSource(seed))
}
func (s *Server) captchaKey(r *http.Request) int { return s.captchaRNG(r).Intn(9) + 1 }
func (s *Server) captchaLocation(r *http.Request, w, h int) (int, int) {
	rng := s.captchaRNG(r)
	// Position the red circle with enough space for the label on the right.
	maxX := w - 175
	maxY := h - 55
	x, y := 22, 22
	if maxX > 22 {
		x += rng.Intn(maxX - 22 + 1)
	}
	if maxY > 22 {
		y += rng.Intn(maxY - 22 + 1)
	}
	return x, y
}
func (s *Server) captcha(w http.ResponseWriter, r *http.Request) {
	from := r.URL.RequestURI()
	body := "<!doctype html><html><head><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>Checking you're not a bot...</title>" +
		"<link rel=\"stylesheet\" href=\"" + s.staticURL("internal.css") + "\"><link rel=\"stylesheet\" href=\"" + s.staticURL("main.css") + "\">" +
		"<script defer src=\"" + s.staticURL("captcha.js") + "\"></script></head>" +
		"<body class=\"skin-fandomdesktop theme-fandomdesktop-light internal\"><div class=\"main-container\"><div class=\"page\"><main class=\"page__main\"><h1>Checking you're not a bot...</h1>" +
		"<div id=\"content\"><p>To confirm, please click directly in the circle, or hold down the " + strconv.Itoa(s.captchaKey(r)) + " key on your keyboard.</p>" +
		"<noscript><p>JavaScript is required for the captcha. Sorry!</p></noscript><div id=\"captcha-area\"></div></div></main></div></div></body></html>"
	_ = from
	s.html(w, 200, body)
}
func (s *Server) captchaImage(w http.ResponseWriter, r *http.Request, m []string) {
	width, _ := strconv.Atoi(m[1])
	height, _ := strconv.Atoi(m[2])
	if width < 60 || height < 60 || width > 8192 || height > 8192 {
		http.Error(w, "Invalid captcha size", 400)
		return
	}
	centerX, centerY := s.captchaLocation(r, width, height)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	white := color.RGBA{255, 255, 255, 255}
	red := color.RGBA{230, 0, 0, 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, white)
		}
	}
	for y := centerY - 18; y <= centerY+18; y++ {
		for x := centerX - 18; x <= centerX+18; x++ {
			if (x-centerX)*(x-centerX)+(y-centerY)*(y-centerY) <= 17*17 {
				img.Set(x, y, red)
			}
		}
	}
	drawer := &font.Drawer{Dst: img, Src: image.NewUniform(color.Black), Face: basicfont.Face7x13,
		Dot: fixed.P(centerX+26, centerY+4)}
	drawer.DrawString("I'm not a robot!")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, img); err != nil {
		fmt.Printf("captcha PNG: %v\n", err)
	}
}
func (s *Server) captchaVerify(w http.ResponseWriter, r *http.Request, m []string) {
	wid, _ := strconv.Atoi(m[1])
	hei, _ := strconv.Atoi(m[2])
	x, _ := strconv.Atoi(m[3])
	y, _ := strconv.Atoi(m[4])
	ok := false
	method := ""
	if y == 0 {
		ok = x == s.captchaKey(r)
		method = "key"
	} else if wid > 0 && hei > 0 {
		cx, cy := s.captchaLocation(r, wid, hei)
		dx, dy := x-cx, y-cy
		ok = dx*dx+dy*dy < 18*18
		method = "coordinate"
	}
	if s.Config.True("captcha::log") {
		fmt.Printf("captcha verify - %s pass=%t [%s]\n", method, ok, s.clientIP(r))
	}
	if ok {
		http.SetCookie(w, &http.Cookie{Name: "captcha", Value: method, Path: "/",
			Secure: method == "key", HttpOnly: method == "key", MaxAge: int((10 * 365 * 24 * time.Hour) / time.Second)})
	}
	from := r.URL.Query().Get("from")
	if from == "" {
		s.notFound(w, r)
		return
	}
	s.common(w)
	w.Header().Set("Location", from)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(302)
	_, _ = io.WriteString(w, "Checking your answer...\n")
}
