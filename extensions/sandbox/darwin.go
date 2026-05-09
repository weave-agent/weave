//go:build darwin

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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
		cfg = SandboxConfig{Mode: ModeAuto, Network: true}
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
	b.WriteString(fmt.Sprintf("(deny file-read* (regex #\"^%s/id_.*$\"))\n", regexp.QuoteMeta(sshDir)))
	b.WriteString(fmt.Sprintf("(deny file-read* (literal %q))\n", filepath.Join(home, ".aws", "credentials")))
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
		b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", w))
	}

	// Mandatory write deny rules (directories)
	denyDirs := []string{
		filepath.Join(home, ".ssh"),
	}
	for _, d := range denyDirs {
		b.WriteString(fmt.Sprintf("(deny file-write* (subpath %q))\n", d))
	}

	// Mandatory write deny rules (files)
	denyFiles := []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".profile"),
		filepath.Join(home, ".gitconfig"),
	}
	for _, f := range denyFiles {
		b.WriteString(fmt.Sprintf("(deny file-write* (literal %q))\n", f))
	}

	b.WriteString(fmt.Sprintf("(deny file-write* (subpath %q))\n", filepath.Join(dir, ".git", "hooks")))
	b.WriteString(fmt.Sprintf("(deny file-write* (literal %q))\n", filepath.Join(dir, ".git", "config")))
	b.WriteString(fmt.Sprintf("(deny file-write* (subpath %q))\n", filepath.Join(dir, ".weave")))

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
