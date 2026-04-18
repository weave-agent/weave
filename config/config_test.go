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
	writeFile(t, dir, ".weave.yaml", "extensions: [noop, logging]\nslots: {runner: turn}\n")

	cf, err := Load(filepath.Join(dir, ".weave.yaml"))
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
}

func TestLoad_SlotsDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\n")

	cf, err := Load(filepath.Join(dir, ".weave.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cf.Slots == nil {
		t.Error("expected non-nil slots map")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/.weave.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
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
