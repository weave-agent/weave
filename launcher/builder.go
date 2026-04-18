package launcher

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ComputeHash returns a deterministic SHA256 hex string for the given extensions.
// The hash covers the Go version and the sorted contents of all .go files.
func ComputeHash(exts []ExtensionInfo) (string, error) {
	h := sha256.New()

	fmt.Fprintf(h, "go%s\n", runtime.Version())

	sorted := make([]ExtensionInfo, len(exts))
	copy(sorted, exts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	for _, ext := range sorted {
		for _, f := range ext.GoFiles {
			data, err := os.ReadFile(f)
			if err != nil {
				return "", fmt.Errorf("hash: read %s: %w", f, err)
			}

			h.Write(data)
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// GenerateGoMod creates a go.mod file for the built binary.
// moduleRoot is the path to the weave module root (containing go.mod).
func GenerateGoMod(dir, moduleRoot string, exts []ExtensionInfo) error {
	var b strings.Builder
	b.WriteString("module weave-built\n\n")
	b.WriteString("go 1.26.2\n\n")
	b.WriteString("require (\n")
	b.WriteString("\tweave v0.0.0\n")

	for _, ext := range exts {
		b.WriteString("\tweave/ext/" + ext.Name + " v0.0.0\n")
	}

	b.WriteString(")\n\n")
	b.WriteString("replace weave => " + moduleRoot + "\n")

	for _, ext := range exts {
		b.WriteString("replace weave/ext/" + ext.Name + " => " + ext.Dir + "\n")
	}

	return os.WriteFile(filepath.Join(dir, "go.mod"), []byte(b.String()), 0o644)
}

// GenerateMainGo creates a main.go with blank imports for each extension.
func GenerateMainGo(dir string, exts []ExtensionInfo) error {
	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t_ \"weave/sdk\"\n")

	for _, ext := range exts {
		b.WriteString("\t_ \"weave/ext/" + ext.Name + "\"\n")
	}

	b.WriteString(")\n\n")
	b.WriteString("func main() {}\n")

	return os.WriteFile(filepath.Join(dir, "main.go"), []byte(b.String()), 0o644)
}

// Build generates go.mod and main.go in dir, runs go build, and returns the binary path.
// moduleRoot is the absolute path to the weave module root (containing go.mod).
func Build(dir, moduleRoot string, exts []ExtensionInfo) (string, error) {
	if err := GenerateGoMod(dir, moduleRoot, exts); err != nil {
		return "", fmt.Errorf("build: generate go.mod: %w", err)
	}

	if err := GenerateMainGo(dir, exts); err != nil {
		return "", fmt.Errorf("build: generate main.go: %w", err)
	}

	// Ensure each extension dir has a go.mod so Go treats it as a module
	for _, ext := range exts {
		if err := ensureExtGoMod(ext, moduleRoot); err != nil {
			return "", fmt.Errorf("build: extension %s go.mod: %w", ext.Name, err)
		}
	}

	binaryPath := filepath.Join(dir, "weave")
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")

	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build: go build: %w\n%s", err, output)
	}

	return binaryPath, nil
}

// ensureExtGoMod writes a go.mod in the extension dir if one doesn't already exist.
func ensureExtGoMod(ext ExtensionInfo, moduleRoot string) error {
	goModPath := filepath.Join(ext.Dir, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		return nil
	}

	content := "module weave/ext/" + ext.Name + "\n\ngo 1.26.2\n\nrequire weave v0.0.0\n\nreplace weave => " + moduleRoot + "\n"

	return os.WriteFile(goModPath, []byte(content), 0o644)
}
