// Package config reads BreezeWiki's config.ini and BW_ environment overrides.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

var defaults = map[string]string{
	"application_name":           "BreezeWiki",
	"bind_host":                  "auto",
	"port":                       "10416",
	"canonical_origin":           "",
	"debug":                      "false",
	"feature_search_suggestions": "true",
	"instance_is_official":       "false",
	"log_outgoing":               "true",
	"strict_proxy":               "false",
	"feature_jsonp::enabled":     "true",
	"access_log::enabled":        "false",
}

// Config contains the effective string values. Like upstream, every value is
// a string, and only the empty string and "false" mean false.
type Config struct{ values map[string]string }

func New(values map[string]string) *Config {
	c := &Config{values: make(map[string]string, len(defaults)+len(values))}
	for k, v := range defaults {
		c.values[k] = v
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
