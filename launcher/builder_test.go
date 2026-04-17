package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeHash_Deterministic(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "ext.go")
	if err := os.WriteFile(f1, []byte("package noop"), 0o644); err != nil {
		t.Fatal(err)
	}

	exts := []ExtensionInfo{
		{Name: "alpha", Dir: dir, GoFiles: []string{f1}},
	}

	h1, err := ComputeHash(exts)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := ComputeHash(exts)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("hash not deterministic: %s != %s", h1, h2)
	}
}

func TestComputeHash_SortedByName(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.go")
	f2 := filepath.Join(dir, "b.go")
	os.WriteFile(f1, []byte("package a"), 0o644)
	os.WriteFile(f2, []byte("package b"), 0o644)

	exts1 := []ExtensionInfo{
		{Name: "alpha", Dir: dir, GoFiles: []string{f1}},
		{Name: "beta", Dir: dir, GoFiles: []string{f2}},
	}
	exts2 := []ExtensionInfo{
		{Name: "beta", Dir: dir, GoFiles: []string{f2}},
		{Name: "alpha", Dir: dir, GoFiles: []string{f1}},
	}

	h1, _ := ComputeHash(exts1)
	h2, _ := ComputeHash(exts2)
	if h1 != h2 {
		t.Errorf("hash should be order-independent: %s != %s", h1, h2)
	}
}

func TestComputeHash_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.go")
	f2 := filepath.Join(dir, "b.go")
	os.WriteFile(f1, []byte("package a"), 0o644)
	os.WriteFile(f2, []byte("package different"), 0o644)

	h1, _ := ComputeHash([]ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f1}}})
	h2, _ := ComputeHash([]ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f2}}})
	if h1 == h2 {
		t.Error("different content should produce different hash")
	}
}

func TestComputeHash_ReadError(t *testing.T) {
	exts := []ExtensionInfo{
		{Name: "x", Dir: "/nonexistent", GoFiles: []string{"/nonexistent/missing.go"}},
	}
	_, err := ComputeHash(exts)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestGenerateGoMod_Content(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "noop", Dir: "/tmp/exts/noop"},
	}
	err := GenerateGoMod(dir, "/tmp/weave", exts)
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}

	s := string(content)
	if !strings.Contains(s, "module weave-built") {
		t.Error("go.mod missing module declaration")
	}
	if !strings.Contains(s, "go 1.26.2") {
		t.Error("go.mod missing go version")
	}
	if !strings.Contains(s, "weave v0.0.0") {
		t.Error("go.mod missing weave require")
	}
	if !strings.Contains(s, "weave/ext/noop v0.0.0") {
		t.Error("go.mod missing extension require")
	}
	if !strings.Contains(s, "replace weave => /tmp/weave") {
		t.Error("go.mod missing module replace")
	}
	if !strings.Contains(s, "replace weave/ext/noop => /tmp/exts/noop") {
		t.Error("go.mod missing extension replace")
	}
}

func TestGenerateMainGo_Content(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "noop", Dir: "/tmp/exts/noop"},
		{Name: "log", Dir: "/tmp/exts/log"},
	}
	err := GenerateMainGo(dir, exts)
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}

	s := string(content)
	if !strings.Contains(s, "package main") {
		t.Error("main.go missing package declaration")
	}
	if !strings.Contains(s, `_ "weave/sdk"`) {
		t.Error("main.go missing sdk import")
	}
	if !strings.Contains(s, `_ "weave/ext/noop"`) {
		t.Error("main.go missing noop import")
	}
	if !strings.Contains(s, `_ "weave/ext/log"`) {
		t.Error("main.go missing log import")
	}
	if !strings.Contains(s, "func main()") {
		t.Error("main.go missing main func")
	}
}

func TestBuild_WithTrivialExtension(t *testing.T) {
	moduleRoot, err := findModuleRoot()
	if err != nil {
		t.Skipf("cannot locate module root: %v", err)
	}

	buildDir := t.TempDir()
	extDir := t.TempDir()

	extCode := `package noop

import "weave/sdk"

func init() {
	sdk.RegisterExtension("noop", func() sdk.Extension {
		return sdk.NewExtensionFunc("noop", func(b sdk.Bus) {})
	})
}
`
	if err := os.WriteFile(filepath.Join(extDir, "noop.go"), []byte(extCode), 0o644); err != nil {
		t.Fatal(err)
	}

	exts := []ExtensionInfo{
		{Name: "noop", Dir: extDir, GoFiles: []string{filepath.Join(extDir, "noop.go")}},
	}

	binaryPath, err := Build(buildDir, moduleRoot, exts)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if _, err := os.Stat(binaryPath); err != nil {
		t.Errorf("binary not found at %s: %v", binaryPath, err)
	}
}

func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", os.ErrNotExist
}
