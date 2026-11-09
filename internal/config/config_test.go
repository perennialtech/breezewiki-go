package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultsINIAndEnvironment(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "config.ini")
	if err := os.WriteFile(filename, []byte("application_name = Example\n[feature_jsonp]\nenabled = false\n[access_log]\nenabled = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BW_APPLICATION_NAME", "Environment")
	t.Setenv("BW_feature_jsonp::enabled", "true")
	c, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if v := c.Get("application_name"); v != "Environment" {
		t.Errorf("env precedence: %q", v)
	}
	if !c.True("feature_jsonp::enabled") {
		t.Error("environment override")
	}
	if !c.True("access_log::enabled") {
		t.Error("section setting")
	}
}
func TestMissingFileAndFalseSemantics(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "absent.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Get("port") != "10416" {
		t.Fatal("missing-file defaults")
	}
	c = New(map[string]string{"debug": "false", "strict_proxy": "", "feature_search_suggestions": "False"})
	if c.True("debug") || c.True("strict_proxy") {
		t.Fatal("false values")
	}
	if !c.True("feature_search_suggestions") {
		t.Fatal("only lowercase false is false")
	}
}
func TestInvalidINI(t *testing.T) {
	file := filepath.Join(t.TempDir(), "broken.ini")
	if err := os.WriteFile(file, []byte("[section\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(file); err == nil {
		t.Fatal("expected syntax error")
	}
}

func TestHelpListsEveryDefault(t *testing.T) {
	help := Help()
	if !strings.Contains(help, "INI settings use [section]") || !strings.Contains(help, "section::key") {
		t.Error("missing section syntax explanation")
	}
	for _, opt := range options {
		if opt.description == "" || !strings.Contains(help, opt.key+" (default: ") {
			t.Errorf("setting %q missing description or default", opt.key)
		}
	}
}
