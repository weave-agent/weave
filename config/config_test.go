package config

import (
	"os"
	"path/filepath"
	"testing"
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
	writeFile(t, dir, ".weave.yaml", "extensions: [noop, logging]\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cf.Extensions) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(cf.Extensions))
	}

	if cf.Extensions[0] != "noop" || cf.Extensions[1] != "logging" {
		t.Errorf("got extensions %v", cf.Extensions)
	}
}

func TestLoad_CoreDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cf.Core.AgentLoop != "loop" {
		t.Errorf("expected default agent-loop 'loop', got %q", cf.Core.AgentLoop)
	}

	if len(cf.Core.Providers) != 1 || cf.Core.Providers[0] != "anthropic" {
		t.Errorf("expected default providers [anthropic], got %v", cf.Core.Providers)
	}
}

func TestLoad_CoreOverride(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "core:\n  agent_loop: custom-loop\n  providers:\n    - openai\n    - google\nextensions:\n  - bash\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cf.Core.AgentLoop != "custom-loop" {
		t.Errorf("expected agent-loop 'custom-loop', got %q", cf.Core.AgentLoop)
	}

	if len(cf.Core.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(cf.Core.Providers))
	}

	if cf.Core.Providers[0] != "openai" || cf.Core.Providers[1] != "google" {
		t.Errorf("got providers %v", cf.Core.Providers)
	}

	if len(cf.Extensions) != 1 || cf.Extensions[0] != "bash" {
		t.Errorf("got extensions %v", cf.Extensions)
	}
}

func TestLoad_CoreExts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "core:\n  providers:\n    - anthropic\n    - openai\nextensions:\n  - bash\n  - file\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coreExts, optExts := cf.CoreExts()

	expectedCore := []string{"loop", "anthropic", "openai"}
	if len(coreExts) != len(expectedCore) {
		t.Fatalf("expected %d core exts, got %d: %v", len(expectedCore), len(coreExts), coreExts)
	}
	for i, name := range expectedCore {
		if coreExts[i] != name {
			t.Errorf("coreExts[%d] = %q, want %q", i, coreExts[i], name)
		}
	}

	expectedOpt := []string{"bash", "file"}
	if len(optExts) != len(expectedOpt) {
		t.Fatalf("expected %d optional exts, got %d: %v", len(expectedOpt), len(optExts), optExts)
	}
	for i, name := range expectedOpt {
		if optExts[i] != name {
			t.Errorf("optExts[%d] = %q, want %q", i, optExts[i], name)
		}
	}
}

func TestLoad_CoreExtsDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coreExts, optExts := cf.CoreExts()

	if len(coreExts) != 2 {
		t.Fatalf("expected 2 core exts (loop + anthropic), got %d: %v", len(coreExts), coreExts)
	}
	if coreExts[0] != "loop" || coreExts[1] != "anthropic" {
		t.Errorf("expected [loop, anthropic], got %v", coreExts)
	}

	if len(optExts) != 0 {
		t.Errorf("expected 0 optional exts, got %d: %v", len(optExts), optExts)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, _, _, err := LoadFromDir("/nonexistent", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
