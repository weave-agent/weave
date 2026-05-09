//go:build darwin

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"weave/sdk"
)

func bwrapAvailable() error {
	return nil
}

func wrapCommandDarwin(cmd, dir string) (string, error) {
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		return cmd, nil
	}

	s := getCurrentSandbox()

	var cfg SandboxConfig

	if s != nil {
		s.mu.RLock()
		cfg = s.cfg
		s.mu.RUnlock()
	} else {
		cfg = SandboxConfig{Mode: sdk.SandboxAuto, Network: true}
	}

	profile := generateSeatbeltProfile(cfg, dir)
	escaped := strings.ReplaceAll(profile, "'", "'\\''")

	return fmt.Sprintf("sandbox-exec -p '%s' bash -c '%s'", escaped, strings.ReplaceAll(cmd, "'", "'\\''")), nil
}

func generateSeatbeltProfile(cfg SandboxConfig, dir string) string {
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")
	b.WriteString("(allow file-read*)\n")

	home, _ := os.UserHomeDir()

	// Mandatory read deny rules
	sshDir := filepath.Join(home, ".ssh")
	fmt.Fprintf(&b, "(deny file-read* (regex #\"^%s/id_.*$\"))\n", regexp.QuoteMeta(sshDir))
	fmt.Fprintf(&b, "(deny file-read* (literal %q))\n", filepath.Join(home, ".aws", "credentials"))
	b.WriteString("(deny file-read* (regex #\"/\\\\.env$\") (regex #\"/\\\\.env\\\\.[^/]+$\"))\n")

	// Writable paths
	if dir == "" {
		dir, _ = os.Getwd()
	}

	writable := cfg.Writable
	if len(writable) == 0 {
		writable = []string{dir}
	}

	for _, w := range writable {
		if w == "." {
			w = dir
		}

		fmt.Fprintf(&b, "(allow file-write* (subpath %q))\n", w)
	}

	// Mandatory write deny rules (directories)
	denyDirs := []string{
		filepath.Join(home, ".ssh"),
	}
	for _, d := range denyDirs {
		fmt.Fprintf(&b, "(deny file-write* (subpath %q))\n", d)
	}

	// Mandatory write deny rules (files)
	denyFiles := []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".profile"),
		filepath.Join(home, ".gitconfig"),
	}
	for _, f := range denyFiles {
		fmt.Fprintf(&b, "(deny file-write* (literal %q))\n", f)
	}

	fmt.Fprintf(&b, "(deny file-write* (subpath %q))\n", filepath.Join(dir, ".git", "hooks"))
	fmt.Fprintf(&b, "(deny file-write* (literal %q))\n", filepath.Join(dir, ".git", "config"))
	fmt.Fprintf(&b, "(deny file-write* (subpath %q))\n", filepath.Join(dir, ".weave"))

	// Process rules
	b.WriteString("(allow process-exec)\n")
	b.WriteString("(allow process-fork)\n")

	// Network rules
	if cfg.Network {
		b.WriteString("(allow network*)\n")
	} else {
		b.WriteString("(deny network*)\n")
	}

	return b.String()
}

func wrapCommandLinux(cmd, dir string) (string, error) {
	return cmd, nil
}

func wrapCommandDarwinWithConfig(cmd, dir string, cfg SandboxConfig) (string, error) {
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		return cmd, nil
	}

	profile := generateSeatbeltProfile(cfg, dir)
	escaped := strings.ReplaceAll(profile, "'", "'\\''")

	return fmt.Sprintf("sandbox-exec -p '%s' bash -c '%s'", escaped, strings.ReplaceAll(cmd, "'", "'\\''")), nil
}

func wrapCommandLinuxWithConfig(cmd, dir string, cfg SandboxConfig) (string, error) {
	return cmd, nil
}
