package launcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ComputeHash returns a deterministic SHA256 hex string for the given extensions.
// The hash covers the Go version, the sorted contents of all extension .go files,
// and the contents of any additional core package directories.
func ComputeHash(exts []ExtensionInfo, coreDirs ...string) (string, error) {
	h := sha256.New()

	h.Write([]byte("go" + runtime.Version() + "\n"))

	sorted := make([]ExtensionInfo, len(exts))
	copy(sorted, exts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	for _, ext := range sorted {
		h.Write([]byte("ext:" + ext.Name + "\n"))

		goModPath := filepath.Join(ext.Dir, "go.mod")
		if data, err := os.ReadFile(goModPath); err == nil {
			h.Write([]byte("go.mod\n"))
			h.Write(data)
		}

		for _, f := range ext.GoFiles {
			rel, relErr := filepath.Rel(ext.Dir, f)
			if relErr != nil {
				rel = f
			}

			h.Write([]byte(rel + "\n"))

			data, err := os.ReadFile(f)
			if err != nil {
				return "", fmt.Errorf("hash: read %s: %w", f, err)
			}

			h.Write(data)
		}
	}

	for _, dir := range coreDirs {
		if err := hashDir(h, dir); err != nil {
			return "", fmt.Errorf("hash: core dir %s: %w", dir, err)
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashDir(h hash.Hash, dir string) error {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("walk dir: %w", err)
	}

	sort.Strings(files)

	for _, f := range files {
		rel, relErr := filepath.Rel(dir, f)
		if relErr != nil {
			rel = f
		}

		h.Write([]byte(rel + "\n"))

		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}

		h.Write(data)
	}

	return nil
}

// goVersion returns the Go version string for go.mod (e.g. "go 1.22.0").
func goVersion() string {
	// runtime.Version() returns e.g. "go1.22.0", "go1.22.0 X:loopvar", "go1.26.2-X:jsonv2"
	v := strings.TrimPrefix(runtime.Version(), "go")
	// Strip anything after a space or dash that isn't a digit/dot
	if idx := strings.IndexFunc(v, func(r rune) bool {
		return !strings.ContainsRune("0123456789.", r)
	}); idx != -1 {
		v = v[:idx]
	}

	return "go " + v
}

// GenerateGoMod creates a go.mod file for the built binary.
// moduleRoot is the path to the weave module root (containing go.mod).
func GenerateGoMod(dir, moduleRoot string, exts []ExtensionInfo) error {
	var b strings.Builder
	b.WriteString("module weave-built\n\n")
	b.WriteString(goVersion() + "\n\n")
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

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("generate go.mod: %w", err)
	}

	return nil
}

// GenerateMainGo creates a main.go that creates a bus, wires all extensions, and blocks on signal.
func GenerateMainGo(dir string, exts []ExtensionInfo) error {
	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"os\"\n")
	b.WriteString("\t\"os/signal\"\n")
	b.WriteString("\t\"syscall\"\n")
	b.WriteString("\n")
	b.WriteString("\t\"weave/bus\"\n")
	b.WriteString("\t\"weave/sdk\"\n")

	for _, ext := range exts {
		b.WriteString("\n")
		b.WriteString("\t_ \"weave/ext/" + ext.Name + "\"\n")
	}

	b.WriteString(")\n\n")
	b.WriteString("func main() {\n")
	b.WriteString("\tb := bus.New()\n")
	b.WriteString("\tdefer b.Close()\n\n")

	extNames := make([]string, len(exts))
	for i, ext := range exts {
		extNames[i] = `"` + ext.Name + `"`
	}

	b.WriteString("\twired, err := sdk.Wire([]string{" + strings.Join(extNames, ", ") + "}, b, nil)\n")
	b.WriteString("\tif err != nil {\n")
	b.WriteString("\t\tfmt.Fprintf(os.Stderr, \"weave: %v\\n\", err)\n")
	b.WriteString("\t\tos.Exit(1)\n")
	b.WriteString("\t}\n\n")
	b.WriteString("\tsig := make(chan os.Signal, 1)\n")
	b.WriteString("\tsignal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)\n")
	b.WriteString("\t<-sig\n")
	b.WriteString("\twired.Close()\n")
	b.WriteString("}\n")

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("generate main.go: %w", err)
	}

	return nil
}

// Build generates go.mod and main.go in dir, runs go build, and returns the binary path.
// moduleRoot is the absolute path to the weave module root (containing go.mod).
// Extensions are sorted by name to match the hash computation order.
func Build(dir, moduleRoot string, exts []ExtensionInfo) (string, error) {
	sort.Slice(exts, func(i, j int) bool {
		return exts[i].Name < exts[j].Name
	})

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
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", binaryPath, ".")

	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build: go build: %w\n%s", err, output)
	}

	return binaryPath, nil
}

// ensureExtGoMod writes a go.mod in the extension dir only if one does not
// already exist. Extensions that ship their own go.mod (with external
// dependencies, toolchain directives, etc.) are left intact.
// Shim go.mods (identified by a sentinel comment) are always regenerated
// so the absolute replace directive stays current after repo moves.
const shimSentinel = "// Code generated by weave launcher. DO NOT EDIT.\n"

func ensureExtGoMod(ext ExtensionInfo, moduleRoot string) error {
	goModPath := filepath.Join(ext.Dir, "go.mod")

	if data, err := os.ReadFile(goModPath); err == nil {
		s := string(data)
		hasSentinel := strings.HasPrefix(s, shimSentinel)
		isWeaveShim := strings.Contains(s, "module weave/ext/"+ext.Name)

		if !hasSentinel && !isWeaveShim {
			return nil
		}
	}

	content := shimSentinel + "module weave/ext/" + ext.Name + "\n\n" + goVersion() + "\n\nrequire weave v0.0.0\n\nreplace weave => " + moduleRoot + "\n"

	if err := os.WriteFile(goModPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("ensure extension go.mod: %w", err)
	}

	return nil
}
