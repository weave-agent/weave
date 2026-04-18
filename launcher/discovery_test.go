package launcher

import (
	"os"
	"path/filepath"
	"testing"
)

func createGoFile(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", filepath.Join(dir, name), err)
	}
}

func TestDiscover_LocalExtension(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, extDir, "noop.go", "package noop")

	exts, err := Discover(projectDir, []string{"noop"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(exts) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(exts))
	}

	if exts[0].Name != "noop" {
		t.Errorf("Name = %q, want %q", exts[0].Name, "noop")
	}

	if exts[0].Dir != extDir {
		t.Errorf("Dir = %q, want %q", exts[0].Dir, extDir)
	}

	if len(exts[0].GoFiles) != 1 {
		t.Errorf("GoFiles count = %d, want 1", len(exts[0].GoFiles))
	}
}

func TestDiscover_GlobalExtension(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	extDir := filepath.Join(homeDir, ".weave", "extensions", "logging")
	createGoFile(t, extDir, "logging.go", "package logging")

	// Override homeDir by calling findExtension directly
	info, err := findExtension(projectDir, homeDir, "logging")
	if err != nil {
		t.Fatalf("findExtension: %v", err)
	}

	if info.Name != "logging" {
		t.Errorf("Name = %q, want %q", info.Name, "logging")
	}

	if info.Dir != extDir {
		t.Errorf("Dir = %q, want %q", info.Dir, extDir)
	}
}

func TestDiscover_LocalPreferredOverGlobal(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	localDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, localDir, "noop.go", "package noop")

	globalDir := filepath.Join(homeDir, ".weave", "extensions", "noop")
	createGoFile(t, globalDir, "noop.go", "package noop")

	info, err := findExtension(projectDir, homeDir, "noop")
	if err != nil {
		t.Fatalf("findExtension: %v", err)
	}

	if info.Dir != localDir {
		t.Errorf("expected local dir %q, got %q", localDir, info.Dir)
	}
}

func TestDiscover_MissingExtension(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	_, err := findExtension(projectDir, homeDir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing extension")
	}
}

func TestDiscover_MultipleExtensions(t *testing.T) {
	projectDir := t.TempDir()
	for _, name := range []string{"noop", "logging"} {
		extDir := filepath.Join(projectDir, ".weave", "extensions", name)
		createGoFile(t, extDir, name+".go", "package "+name)
	}

	exts, err := Discover(projectDir, []string{"noop", "logging"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(exts) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(exts))
	}

	names := []string{exts[0].Name, exts[1].Name}
	if names[0] != "noop" || names[1] != "logging" {
		t.Errorf("names = %v, want [noop logging]", names)
	}
}

func TestDiscover_EmptyExtensionDir(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	extDir := filepath.Join(projectDir, ".weave", "extensions", "empty")
	if err := os.MkdirAll(extDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := findExtension(projectDir, homeDir, "empty")
	if err == nil {
		t.Fatal("expected error for extension dir with no .go files")
	}
}

func TestDiscover_GoFilesSorted(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "sorted")
	createGoFile(t, extDir, "z.go", "package sorted")
	createGoFile(t, extDir, "a.go", "package sorted")
	createGoFile(t, extDir, "m.go", "package sorted")

	exts, err := Discover(projectDir, []string{"sorted"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	expected := []string{
		filepath.Join(extDir, "a.go"),
		filepath.Join(extDir, "m.go"),
		filepath.Join(extDir, "z.go"),
	}
	if len(exts[0].GoFiles) != len(expected) {
		t.Fatalf("GoFiles count = %d, want %d", len(exts[0].GoFiles), len(expected))
	}

	for i, f := range exts[0].GoFiles {
		if f != expected[i] {
			t.Errorf("GoFiles[%d] = %q, want %q", i, f, expected[i])
		}
	}
}

func TestDiscover_PartialMissing(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "exists")
	createGoFile(t, extDir, "exists.go", "package exists")

	_, err := Discover(projectDir, []string{"exists", "missing"})
	if err == nil {
		t.Fatal("expected error when one extension is missing")
	}
}
