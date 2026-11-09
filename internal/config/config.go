// Package config reads BreezeWiki's config.ini and BW_ environment overrides.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// option is an INI key and its built-in default and description.
type option struct {
	key, value, description string
}

// options is the source of truth for the supported settings and their help.
var options = []option{
	{"application_name", "BreezeWiki", "Name displayed in page footers and startup logs."},
	{"bind_host", "auto", "Listening address. auto binds all interfaces, or loopback when debug is true."},
	{"port", "10416", "TCP port for the HTTP server (1-65535)."},
	{"canonical_origin", "", "Public URL for canonical links and subdomain redirects. Empty disables those features; POST origin checks then use the request host."},
	{"debug", "false", "Disable static asset caching and bind loopback when bind_host is auto."},
	{"feature_search_suggestions", "true", "Enable the client-side Fandom search suggestion widget."},
	{"instance_is_official", "false", "Show the original BreezeWiki developer attribution as the instance operator."},
	{"log_outgoing", "true", "Log outgoing Fandom API requests."},
	{"strict_proxy", "false", "Route supported external image, stylesheet, and icon URLs through the local /proxy endpoint."},
	{"feature_jsonp::enabled", "true", "Load wiki data in the browser instead of rendering it server-side on GET."},
	{"access_log::enabled", "false", "Log incoming HTTP requests."},
}

// Config contains the effective string values. Like upstream, every value is
// a string, and only the empty string and "false" mean false.
type Config struct{ values map[string]string }

func New(values map[string]string) *Config {
	c := &Config{values: make(map[string]string, len(options)+len(values))}
	for _, opt := range options {
		c.values[opt.key] = opt.value
	}
	for k, v := range values {
		c.values[strings.ToLower(k)] = v
	}
	return c
}

func (c *Config) Get(key string) string { return c.values[strings.ToLower(key)] }
func (c *Config) True(key string) bool {
	v := c.Get(key)
	return v != "" && v != "false"
}

// Load reads an optional INI file, then overlays BW_ environment values.
// Root keys and [section] keys follow Racket's 'section::key' convention.
// A missing file is not an error, just as in the original application.
func Load(path string) (*Config, error) {
	if path == "" {
		path = "config.ini"
	}
	values := make(map[string]string)
	file, err := os.Open(path)
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		section := ""
		for line := 1; scanner.Scan(); line++ {
			raw := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
			if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, ";") {
				continue
			}
			if strings.HasPrefix(raw, "[") {
				if !strings.HasSuffix(raw, "]") {
					return nil, fmt.Errorf("%s:%d: invalid section", path, line)
				}
				section = strings.ToLower(strings.TrimSpace(raw[1 : len(raw)-1]))
				continue
			}
			pos := strings.IndexAny(raw, "=:")
			if pos < 1 {
				return nil, fmt.Errorf("%s:%d: invalid setting", path, line)
			}
			key := strings.ToLower(strings.TrimSpace(raw[:pos]))
			value := strings.TrimSpace(raw[pos+1:])
			if n := len(value); n >= 2 && (value[0] == '"' && value[n-1] == '"' || value[0] == '\'' && value[n-1] == '\'') {
				value = value[1 : n-1]
			}
			if section != "" {
				key = section + "::" + key
			}
			values[key] = value
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, entry := range os.Environ() {
		k, v, ok := strings.Cut(entry, "=")
		if ok && len(k) >= 3 && strings.EqualFold(k[:3], "BW_") {
			values[strings.ToLower(k[3:])] = v
		}
	}
	return New(values), nil
}

// Help lists supported INI settings and their effective default values.
func Help() string {
	var b strings.Builder
	b.WriteString("INI settings use [section] and key = value. The names below use section::key notation for nested settings and BW_ environment overrides.\n\n")
	for _, opt := range options {
		fmt.Fprintf(&b, "%s (default: %q)\n  %s\n", opt.key, opt.value, opt.description)
	}
	return b.String()
}
