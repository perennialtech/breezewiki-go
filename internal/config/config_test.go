package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsINIAndEnvironment(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "config.ini")
	if err := os.WriteFile(filename, []byte("application_name = Example\n[feature_offline]\nenabled = true\nformat = json\n[promotions]\nindie_wiki_buddy = banner footer\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BW_APPLICATION_NAME", "Environment")
	t.Setenv("BW_feature_offline::format", "json.gz")
	c, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if v := c.Get("application_name"); v != "Environment" {
		t.Errorf("env precedence: %q", v)
	}
	if !c.True("feature_offline::enabled") || c.Get("feature_offline::format") != "json.gz" {
		t.Errorf("section and BW_ overrides: %#v", c)
	}
	if !c.Member("promotions::indie_wiki_buddy", "footer") || c.Member("promotions::indie_wiki_buddy", "home") {
		t.Error("membership")
	}
	if !c.True("feature_jsonp::enabled") || c.True("captcha::enabled") {
		t.Error("defaults")
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
	c = New(map[string]string{"debug": "false", "strict_proxy": "", "captcha::enabled": "False"})
	if c.True("debug") || c.True("strict_proxy") {
		t.Fatal("false values")
	}
	if !c.True("captcha::enabled") {
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
