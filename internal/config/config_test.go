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

func TestGeneratedExamplesContainEveryDefault(t *testing.T) {
	ini, err := Example("ini")
	if err != nil {
		t.Fatal(err)
	}
	env, err := Example("env")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "config.ini")
	if err := os.WriteFile(file, []byte(ini), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, opt := range options {
		if got := cfg.Get(opt.key); got != opt.value {
			t.Errorf("INI default %q: got %q, want %q", opt.key, got, opt.value)
		}
		key := opt.key
		if _, name, ok := strings.Cut(key, "::"); ok {
			key = name
		}
		value := opt.value
		if value != "" {
			value = " " + value
		}
		if !strings.Contains(ini, "\n"+key+" ="+value+"\n") {
			t.Errorf("INI example missing %q", opt.key)
		}
		line := "\nBW_" + strings.ToUpper(opt.key) + "=" + opt.value + "\n"
		if count := strings.Count("\n"+env, line); count != 1 {
			t.Errorf("env example: %q occurs %d times", line, count)
		}
	}
	if _, err := Example("yaml"); err == nil {
		t.Error("unsupported format should fail")
	}
}

func TestExampleINIRootKeysAfterSectionedOptions(t *testing.T) {
	previous := options
	options = []option{
		{"access_log::enabled", "false", "Access log."},
		{"port", "10416", "Listening port."},
	}
	t.Cleanup(func() { options = previous })
	ini, err := Example("ini")
	if err != nil {
		t.Fatal(err)
	}
	rootAt := strings.Index(ini, "\nport = 10416\n")
	sectionAt := strings.Index(ini, "\n[access_log]\n")
	if rootAt < 0 || sectionAt < 0 || rootAt >= sectionAt {
		t.Fatalf("root option emitted inside [access_log]:\n%s", ini)
	}
}
