package cfg

import (
	"os"
	"path/filepath"
	"testing"

	"weave/sdk"
)

func TestFindConfigPath_WeaveYaml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: [noop]\n")

	got, err := FindConfigPath(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, ".weave.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFindConfigPath_ConfigDir(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".weave")
	mkdir(t, configDir)
	writeFile(t, configDir, "config.yaml", "extensions: []\n")

	got, err := FindConfigPath(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(configDir, "config.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFindConfigPath_WalkUp(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "a", "b", "c")
	mkdir(t, child)
	writeFile(t, root, ".weave.yaml", "extensions: []\n")

	got, err := FindConfigPath(child)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(root, ".weave.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFindConfigPath_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := FindConfigPath(dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFindConfigPath_PrefersWeaveYaml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: [first]\n")
	configDir := filepath.Join(dir, ".weave")
	mkdir(t, configDir)
	writeFile(t, configDir, "config.yaml", "extensions: [second]\n")

	got, err := FindConfigPath(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(got) != ".weave.yaml" {
		t.Errorf("expected .weave.yaml to be preferred, got %q", got)
	}
}

func TestLoad_Extensions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: [noop, logging]\nslots: {runner: turn}\n")

	cf, cfg, err := Load(filepath.Join(dir, ".weave.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cf.Extensions) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(cf.Extensions))
	}
	if cf.Extensions[0] != "noop" || cf.Extensions[1] != "logging" {
		t.Errorf("got extensions %v", cf.Extensions)
	}
	if cf.Slots["runner"] != "turn" {
		t.Errorf("got slots %v", cf.Slots)
	}

	got := cfg.GetStringSlice("extensions")
	if len(got) != 2 || got[0] != "noop" || got[1] != "logging" {
		t.Errorf("GetStringSlice(extensions) = %v", got)
	}
}

func TestLoad_SlotsDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\n")

	cf, _, err := Load(filepath.Join(dir, ".weave.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cf.Slots == nil {
		t.Error("expected non-nil slots map")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, _, err := Load("/nonexistent/.weave.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestConfig_GetString(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\nslots: {runner: turn}\n")
	_, cfg, _ := Load(filepath.Join(dir, ".weave.yaml"))

	tests := []struct {
		key  string
		want string
	}{
		{"slots.runner", "turn"},
		{"slots.missing", ""},
		{"missing.key", ""},
	}
	for _, tt := range tests {
		got := cfg.GetString(tt.key)
		if got != tt.want {
			t.Errorf("GetString(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestConfig_GetInt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\nport: 8080\n")
	_, cfg, _ := Load(filepath.Join(dir, ".weave.yaml"))

	if got := cfg.GetInt("port"); got != 8080 {
		t.Errorf("GetInt(port) = %d, want 8080", got)
	}
	if got := cfg.GetInt("missing"); got != 0 {
		t.Errorf("GetInt(missing) = %d, want 0", got)
	}
}

func TestConfig_GetBool(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\ndebug: true\nverbose: false\n")
	_, cfg, _ := Load(filepath.Join(dir, ".weave.yaml"))

	if got := cfg.GetBool("debug"); !got {
		t.Error("GetBool(debug) = false, want true")
	}
	if got := cfg.GetBool("verbose"); got {
		t.Error("GetBool(verbose) = true, want false")
	}
	if got := cfg.GetBool("missing"); got {
		t.Error("GetBool(missing) = true, want false")
	}
}

func TestConfig_GetStringSlice(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: [a, b, c]\n")
	_, cfg, _ := Load(filepath.Join(dir, ".weave.yaml"))

	got := cfg.GetStringSlice("extensions")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("GetStringSlice = %v", got)
	}
	if got := cfg.GetStringSlice("missing"); got != nil {
		t.Errorf("GetStringSlice(missing) = %v, want nil", got)
	}
}

func TestConfig_Sub(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\nprovider:\n  model: gpt-4\n  temp: 0.7\n")
	_, cfg, _ := Load(filepath.Join(dir, ".weave.yaml"))

	sub := cfg.Sub("provider")
	if sub == nil {
		t.Fatal("Sub(provider) returned nil")
	}
	if got := sub.GetString("model"); got != "gpt-4" {
		t.Errorf("Sub(provider).GetString(model) = %q, want %q", got, "gpt-4")
	}
	if got := sub.GetInt("temp"); got != 0 {
		t.Errorf("Sub(provider).GetInt(temp) = %d, want 0 (float64 truncation)", got)
	}

	missing := cfg.Sub("missing")
	if got := missing.GetString("anything"); got != "" {
		t.Errorf("Sub(missing).GetString = %q, want empty", got)
	}
}

func TestConfig_InterfaceSatisfaction(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\n")
	_, cfg, _ := Load(filepath.Join(dir, ".weave.yaml"))

	var _ sdk.Config = cfg
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
